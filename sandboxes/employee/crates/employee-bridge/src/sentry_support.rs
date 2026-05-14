use std::borrow::Cow;
use std::io::{Read, Write};
use std::net::{TcpStream, ToSocketAddrs};
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::Duration;

use sentry::transports::DefaultTransportFactory;
use sentry::{ClientOptions, Envelope, Transport, TransportFactory};
use tracing_subscriber::layer::SubscriberExt;
use tracing_subscriber::util::SubscriberInitExt;
use tracing_subscriber::EnvFilter;

const DEFAULT_SPOTLIGHT_URL: &str = "http://localhost:8969/stream";
const SPOTLIGHT_ONLY_DSN: &str = "http://public@localhost/1";
const MAX_SPOTLIGHT_FAILURES: usize = 3;

#[derive(Debug, Clone, PartialEq)]
struct SentryConfig {
    dsn: Option<String>,
    environment: String,
    release: Option<String>,
    sample_rate: f32,
    traces_sample_rate: f32,
    enable_logs: bool,
    debug: bool,
    spotlight_url: Option<String>,
}

impl SentryConfig {
    fn from_env() -> Self {
        let spotlight_enabled = bool_env("SENTRY_SPOTLIGHT").unwrap_or(false);
        Self {
            dsn: non_empty_env("SENTRY_DSN"),
            environment: non_empty_env("SENTRY_ENVIRONMENT")
                .or_else(|| non_empty_env("APP_ENV"))
                .or_else(|| non_empty_env("RUST_ENV"))
                .unwrap_or_else(|| "development".to_string()),
            release: non_empty_env("SENTRY_RELEASE"),
            sample_rate: f32_env("SENTRY_SAMPLE_RATE").unwrap_or(1.0),
            traces_sample_rate: f32_env("SENTRY_TRACES_SAMPLE_RATE")
                .unwrap_or(if spotlight_enabled { 1.0 } else { 0.0 }),
            enable_logs: bool_env("SENTRY_ENABLE_LOGS").unwrap_or(spotlight_enabled),
            debug: bool_env("SENTRY_DEBUG").unwrap_or(false),
            spotlight_url: if spotlight_enabled {
                Some(
                    non_empty_env("SENTRY_SPOTLIGHT_URL")
                        .unwrap_or_else(|| DEFAULT_SPOTLIGHT_URL.to_string()),
                )
            } else {
                None
            },
        }
    }

    fn enabled(&self) -> bool {
        self.dsn.is_some() || self.spotlight_url.is_some()
    }
}

pub fn init_sentry() -> Option<sentry::ClientInitGuard> {
    let config = SentryConfig::from_env();
    if !config.enabled() {
        return None;
    }

    let dsn = config
        .dsn
        .as_deref()
        .unwrap_or(SPOTLIGHT_ONLY_DSN)
        .parse()
        .expect("SENTRY_DSN must be a valid Sentry DSN URL");

    let mut options = ClientOptions {
        dsn: Some(dsn),
        environment: Some(Cow::Owned(config.environment.clone())),
        release: config
            .release
            .clone()
            .map(Cow::Owned)
            .or_else(|| sentry::release_name!()),
        sample_rate: config.sample_rate,
        traces_sample_rate: config.traces_sample_rate,
        attach_stacktrace: true,
        send_default_pii: false,
        debug: config.debug,
        enable_logs: config.enable_logs,
        ..Default::default()
    };

    if let Some(spotlight_url) = config.spotlight_url.clone() {
        options.transport = Some(Arc::new(SpotlightTransportFactory {
            spotlight_url,
            forward_to_sentry: config.dsn.is_some(),
            debug: config.debug,
        }));
    }

    let guard = sentry::init(options);
    sentry::configure_scope(|scope| {
        scope.set_tag("service", "employee-bridge");
        for (key, value) in runtime_sentry_tags() {
            scope.set_tag(key, value);
        }
        if config.spotlight_url.is_some() {
            scope.set_tag("sentry.spotlight", "true");
        }
    });

    Some(guard)
}

pub fn init_tracing(sentry_enabled: bool) {
    let env_filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));
    let fmt_layer = tracing_subscriber::fmt::layer();
    let subscriber = tracing_subscriber::registry()
        .with(env_filter)
        .with(fmt_layer);

    if sentry_enabled {
        subscriber
            .with(sentry::integrations::tracing::layer())
            .init();
    } else {
        subscriber.init();
    }
}

#[derive(Clone)]
struct SpotlightTransportFactory {
    spotlight_url: String,
    forward_to_sentry: bool,
    debug: bool,
}

impl TransportFactory for SpotlightTransportFactory {
    fn create_transport(&self, options: &ClientOptions) -> Arc<dyn Transport> {
        let upstream = if self.forward_to_sentry {
            Some(DefaultTransportFactory.create_transport(options))
        } else {
            None
        };

        Arc::new(SpotlightTransport {
            upstream,
            spotlight_url: self.spotlight_url.clone(),
            debug: self.debug,
            failures: AtomicUsize::new(0),
        })
    }
}

struct SpotlightTransport {
    upstream: Option<Arc<dyn Transport>>,
    spotlight_url: String,
    debug: bool,
    failures: AtomicUsize,
}

impl Transport for SpotlightTransport {
    fn send_envelope(&self, envelope: Envelope) {
        if let Some(upstream) = &self.upstream {
            upstream.send_envelope(envelope.clone());
        }

        if self.failures.load(Ordering::Relaxed) >= MAX_SPOTLIGHT_FAILURES {
            return;
        }

        let mut body = Vec::new();
        if let Err(error) = envelope.to_writer(&mut body) {
            self.record_failure(format!("serialize envelope: {error}"));
            return;
        }

        match post_spotlight_envelope(&self.spotlight_url, &body) {
            Ok(status) if (200..400).contains(&status) => {
                self.failures.store(0, Ordering::Relaxed);
            }
            Ok(status) => {
                self.record_failure(format!("HTTP {status}"));
            }
            Err(error) => {
                self.record_failure(error);
            }
        }
    }

    fn flush(&self, timeout: Duration) -> bool {
        self.upstream
            .as_ref()
            .map(|transport| transport.flush(timeout))
            .unwrap_or(true)
    }

    fn shutdown(&self, timeout: Duration) -> bool {
        self.upstream
            .as_ref()
            .map(|transport| transport.shutdown(timeout))
            .unwrap_or(true)
    }
}

fn post_spotlight_envelope(url: &str, body: &[u8]) -> Result<u16, String> {
    let url = url::Url::parse(url).map_err(|error| format!("invalid Spotlight URL: {error}"))?;
    if url.scheme() != "http" {
        return Err("SENTRY_SPOTLIGHT_URL must use http://".to_string());
    }
    let host = url
        .host_str()
        .ok_or_else(|| "SENTRY_SPOTLIGHT_URL must include a host".to_string())?;
    let port = url
        .port_or_known_default()
        .ok_or_else(|| "SENTRY_SPOTLIGHT_URL must include a port".to_string())?;
    let address = (host, port)
        .to_socket_addrs()
        .map_err(|error| format!("resolve Spotlight host: {error}"))?
        .next()
        .ok_or_else(|| "Spotlight host resolved to no addresses".to_string())?;

    let mut path = if url.path().is_empty() {
        "/".to_string()
    } else {
        url.path().to_string()
    };
    if let Some(query) = url.query() {
        path.push('?');
        path.push_str(query);
    }

    let mut stream = TcpStream::connect_timeout(&address, Duration::from_secs(2))
        .map_err(|error| format!("connect Spotlight sidecar: {error}"))?;
    stream
        .set_read_timeout(Some(Duration::from_secs(2)))
        .map_err(|error| format!("set Spotlight read timeout: {error}"))?;
    stream
        .set_write_timeout(Some(Duration::from_secs(2)))
        .map_err(|error| format!("set Spotlight write timeout: {error}"))?;

    let request = format!(
        "POST {path} HTTP/1.1\r\nhost: {host}:{port}\r\ncontent-type: application/x-sentry-envelope\r\ncontent-length: {}\r\nconnection: close\r\n\r\n",
        body.len()
    );
    stream
        .write_all(request.as_bytes())
        .and_then(|_| stream.write_all(body))
        .map_err(|error| format!("write Spotlight request: {error}"))?;

    let mut response = [0_u8; 64];
    let bytes_read = stream
        .read(&mut response)
        .map_err(|error| format!("read Spotlight response: {error}"))?;
    let response = String::from_utf8_lossy(&response[..bytes_read]);
    let status = response
        .split_whitespace()
        .nth(1)
        .ok_or_else(|| "Spotlight response missing status".to_string())?
        .parse::<u16>()
        .map_err(|error| format!("parse Spotlight status: {error}"))?;
    Ok(status)
}

impl SpotlightTransport {
    fn record_failure(&self, message: String) {
        let failures = self.failures.fetch_add(1, Ordering::Relaxed) + 1;
        if self.debug {
            eprintln!(
                "[sentry-spotlight] failed to send envelope to {} ({failures}/{MAX_SPOTLIGHT_FAILURES}): {message}",
                self.spotlight_url
            );
        }
    }
}

fn non_empty_env(key: &str) -> Option<String> {
    std::env::var(key)
        .ok()
        .filter(|value| !value.trim().is_empty())
}

fn bool_env(key: &str) -> Option<bool> {
    non_empty_env(key).map(|value| matches!(value.as_str(), "1" | "true" | "TRUE" | "yes" | "on"))
}

fn f32_env(key: &str) -> Option<f32> {
    non_empty_env(key).and_then(|value| value.parse().ok())
}

fn runtime_sentry_tags() -> Vec<(&'static str, String)> {
    let mut tags = Vec::new();
    if let Some(org_id) = non_empty_env("HIVELOOP_ORG_ID") {
        tags.push(("org_id", org_id));
    }
    if let Some(agent_id) = non_empty_env("HIVELOOP_AGENT_ID") {
        tags.push(("agent_id", agent_id));
    }
    if let Some(employee_id) = non_empty_env("EMPLOYEE_ID") {
        tags.push(("employee_id", employee_id));
    }
    if let Some(sandbox_id) = non_empty_env("HIVELOOP_SANDBOX_ID") {
        tags.push(("sandbox_id", sandbox_id));
    }
    tags
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::sync::{Mutex, OnceLock};
    use std::thread;
    use std::time::Duration;

    #[test]
    fn sentry_config_enables_spotlight_without_dsn() {
        let _guard = env_lock().lock().unwrap();
        EnvGuard::clear_all();
        std::env::set_var("SENTRY_SPOTLIGHT", "true");

        let config = SentryConfig::from_env();

        assert!(config.enabled());
        assert_eq!(config.dsn, None);
        assert_eq!(config.spotlight_url.as_deref(), Some(DEFAULT_SPOTLIGHT_URL));
        assert_eq!(config.traces_sample_rate, 1.0);
        assert!(config.enable_logs);
    }

    #[test]
    fn sentry_config_uses_explicit_dsn_environment_and_release() {
        let _guard = env_lock().lock().unwrap();
        EnvGuard::clear_all();
        std::env::set_var("SENTRY_DSN", "https://public@example.com/1");
        std::env::set_var("SENTRY_ENVIRONMENT", "staging");
        std::env::set_var("SENTRY_RELEASE", "employee-bridge@test");
        std::env::set_var("SENTRY_TRACES_SAMPLE_RATE", "0.25");

        let config = SentryConfig::from_env();

        assert!(config.enabled());
        assert_eq!(config.dsn.as_deref(), Some("https://public@example.com/1"));
        assert_eq!(config.environment, "staging");
        assert_eq!(config.release.as_deref(), Some("employee-bridge@test"));
        assert_eq!(config.traces_sample_rate, 0.25);
        assert!(!config.enable_logs);
    }

    #[test]
    fn runtime_sentry_tags_use_hiveloop_identity_env() {
        let _guard = env_lock().lock().unwrap();
        EnvGuard::clear_all();
        std::env::set_var("HIVELOOP_ORG_ID", "org-123");
        std::env::set_var("HIVELOOP_AGENT_ID", "agent-123");
        std::env::set_var("EMPLOYEE_ID", "employee-123");
        std::env::set_var("HIVELOOP_SANDBOX_ID", "sandbox-123");

        let tags = runtime_sentry_tags();

        assert!(tags.contains(&("org_id", "org-123".to_string())));
        assert!(tags.contains(&("agent_id", "agent-123".to_string())));
        assert!(tags.contains(&("employee_id", "employee-123".to_string())));
        assert!(tags.contains(&("sandbox_id", "sandbox-123".to_string())));
    }

    #[test]
    fn spotlight_transport_posts_serialized_envelope() {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let url = format!("http://{}/stream", listener.local_addr().unwrap());
        let handle = thread::spawn(move || {
            let (mut stream, _) = listener.accept().unwrap();
            stream
                .set_read_timeout(Some(Duration::from_secs(2)))
                .unwrap();
            let mut request = Vec::new();
            let mut buffer = [0_u8; 4096];
            loop {
                let bytes_read = stream.read(&mut buffer).unwrap();
                request.extend_from_slice(&buffer[..bytes_read]);
                if request.windows(4).any(|window| window == b"\r\n\r\n") {
                    break;
                }
            }
            let headers = String::from_utf8_lossy(&request).to_string();
            let content_length = headers
                .lines()
                .find_map(|line| {
                    line.to_ascii_lowercase()
                        .strip_prefix("content-length: ")
                        .and_then(|value| value.trim().parse::<usize>().ok())
                })
                .unwrap();
            let header_end = request
                .windows(4)
                .position(|window| window == b"\r\n\r\n")
                .map(|idx| idx + 4)
                .unwrap();
            while request.len() - header_end < content_length {
                let bytes_read = stream.read(&mut buffer).unwrap();
                request.extend_from_slice(&buffer[..bytes_read]);
            }
            stream
                .write_all(b"HTTP/1.1 200 OK\r\ncontent-length: 0\r\n\r\n")
                .unwrap();
            String::from_utf8_lossy(&request).to_string()
        });

        let transport = SpotlightTransport {
            upstream: None,
            spotlight_url: url,
            debug: true,
            failures: AtomicUsize::new(0),
        };
        let mut envelope = Envelope::new();
        envelope.add_item(sentry::protocol::Event {
            message: Some("spotlight smoke test".into()),
            ..Default::default()
        });

        transport.send_envelope(envelope);

        let request = handle.join().unwrap();
        assert!(request.starts_with("POST /stream HTTP/1.1"));
        assert!(request
            .to_ascii_lowercase()
            .contains("content-type: application/x-sentry-envelope"));
        assert!(request.contains("spotlight smoke test"));
    }

    struct EnvGuard;

    impl EnvGuard {
        fn clear_all() {
            for key in [
                "SENTRY_DSN",
                "SENTRY_ENVIRONMENT",
                "APP_ENV",
                "RUST_ENV",
                "SENTRY_RELEASE",
                "SENTRY_SAMPLE_RATE",
                "SENTRY_TRACES_SAMPLE_RATE",
                "SENTRY_ENABLE_LOGS",
                "SENTRY_DEBUG",
                "SENTRY_SPOTLIGHT",
                "SENTRY_SPOTLIGHT_URL",
                "HIVELOOP_ORG_ID",
                "HIVELOOP_AGENT_ID",
                "EMPLOYEE_ID",
                "HIVELOOP_SANDBOX_ID",
            ] {
                std::env::remove_var(key);
            }
        }
    }

    fn env_lock() -> &'static Mutex<()> {
        static LOCK: OnceLock<Mutex<()>> = OnceLock::new();
        LOCK.get_or_init(|| Mutex::new(()))
    }
}
