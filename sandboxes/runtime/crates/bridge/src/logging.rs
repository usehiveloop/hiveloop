use bridge_core::RuntimeConfig;
use tokio_util::sync::CancellationToken;
use tracing::info;

/// Initialize tracing/logging based on configuration.
/// When `BRIDGE_OTEL_ENDPOINT` is set, adds an OpenTelemetry layer that exports
/// spans via OTLP gRPC — all existing `tracing` spans become OTel spans.
pub(crate) fn init_logging(config: &RuntimeConfig) {
    use tracing_subscriber::layer::SubscriberExt;
    use tracing_subscriber::util::SubscriberInitExt;
    use tracing_subscriber::EnvFilter;

    // Build filter: honour RUST_LOG if set, otherwise use config log_level
    // with sensible defaults to suppress noisy library crates.
    let env_filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| {
        EnvFilter::new(format!(
            "{},rig=warn,h2=info,hyper_util=info,reqwest=info",
            config.log_level
        ))
    });

    // Optionally build OpenTelemetry layer for OTLP trace export
    let otel_layer = if let Some(ref endpoint) = config.otel_endpoint {
        match init_otel_tracer(endpoint, &config.otel_service_name) {
            Ok(tracer) => {
                eprintln!(
                    "OpenTelemetry tracing enabled: endpoint={}, service={}",
                    endpoint, config.otel_service_name
                );
                Some(tracing_opentelemetry::layer().with_tracer(tracer))
            }
            Err(e) => {
                eprintln!("Failed to initialize OpenTelemetry: {e}");
                None
            }
        }
    } else {
        None
    };

    // Compose: registry + env_filter + otel (optional) + fmt
    // OTel layer is added before fmt so it has the same subscriber type param.
    let registry = tracing_subscriber::registry()
        .with(env_filter)
        .with(otel_layer);

    match config.log_format {
        bridge_core::LogFormat::Json => {
            registry
                .with(tracing_subscriber::fmt::layer().json())
                .init();
        }
        bridge_core::LogFormat::Text => {
            registry.with(tracing_subscriber::fmt::layer()).init();
        }
    }
}

/// Initialize the OpenTelemetry OTLP tracer pipeline.
fn init_otel_tracer(
    endpoint: &str,
    service_name: &str,
) -> Result<opentelemetry_sdk::trace::SdkTracer, Box<dyn std::error::Error>> {
    use opentelemetry::trace::TracerProvider as _;
    use opentelemetry_otlp::WithExportConfig;
    use opentelemetry_sdk::trace::SdkTracerProvider;
    use opentelemetry_sdk::Resource;

    let exporter = opentelemetry_otlp::SpanExporter::builder()
        .with_tonic()
        .with_endpoint(endpoint)
        .build()?;

    let provider = SdkTracerProvider::builder()
        .with_simple_exporter(exporter)
        .with_resource(
            Resource::builder()
                .with_service_name(service_name.to_string())
                .build(),
        )
        .build();

    let tracer = provider.tracer("bridge");

    // Set as global provider so shutdown can flush
    opentelemetry::global::set_tracer_provider(provider);

    Ok(tracer)
}

/// Wait for a shutdown signal (SIGTERM, SIGINT, or cancellation token).
pub(crate) async fn shutdown_signal(cancel: CancellationToken) {
    let ctrl_c = async {
        tokio::signal::ctrl_c()
            .await
            .expect("failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .expect("failed to install signal handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => info!("received SIGINT"),
        _ = terminate => info!("received SIGTERM"),
        _ = cancel.cancelled() => info!("cancellation requested"),
    }
}
