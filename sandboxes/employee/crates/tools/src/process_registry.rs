use dashmap::DashMap;
use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use tokio::process::Command;
use tokio::io::{AsyncBufReadExt, BufReader};

pub struct BackgroundProcessStatus {
    pub running: bool,
    pub exit_code: Option<i32>,
    pub output: String,
}

struct BackgroundProcess {
    status: Arc<Mutex<BackgroundProcessStatus>>,
    started_at: std::time::Instant,
}

pub struct ProcessRegistry {
    processes: DashMap<String, BackgroundProcess>,
}

impl ProcessRegistry {
    pub fn new() -> Self {
        Self { processes: DashMap::new() }
    }

    pub fn spawn(&self, command: &str, env: HashMap<String, String>, timeout_seconds: u64) -> String {
        let process_id = format!("bash-{}", chrono::Utc::now().timestamp_millis());
        let status = Arc::new(Mutex::new(BackgroundProcessStatus {
            running: true,
            exit_code: None,
            output: String::new(),
        }));

        let status_clone = status.clone();
        let command_owned = command.to_string();

        self.processes.insert(process_id.clone(), BackgroundProcess {
            status: status_clone,
            started_at: std::time::Instant::now(),
        });

        tokio::spawn(run_background_process(command_owned, env, status, timeout_seconds));
        process_id
    }

    pub fn status(&self, process_id: &str) -> Option<BackgroundProcessStatus> {
        let entry = self.processes.get(process_id)?;
        if entry.started_at.elapsed() > Duration::from_secs(1800) {
            drop(entry);
            self.processes.remove(process_id);
            return None;
        }
        let inner = entry.status.lock().unwrap();
        Some(BackgroundProcessStatus {
            running: inner.running,
            exit_code: inner.exit_code,
            output: inner.output.clone(),
        })
    }
}

async fn run_background_process(
    command: String,
    env: HashMap<String, String>,
    status: Arc<Mutex<BackgroundProcessStatus>>,
    timeout_seconds: u64,
) {
    let mut cmd = if cfg!(target_os = "windows") {
        let mut c = Command::new("cmd");
        c.arg("/C").arg(&command);
        c
    } else {
        let mut c = Command::new("bash");
        c.arg("-c").arg(&command);
        c
    };
    cmd.kill_on_drop(true);
    for (k, v) in &env { cmd.env(k, v); }
    cmd.stdout(std::process::Stdio::piped());
    cmd.stderr(std::process::Stdio::piped());

    let mut child = match cmd.spawn() {
        Ok(c) => c,
        Err(e) => {
            let mut s = status.lock().unwrap();
            s.running = false;
            s.exit_code = Some(-1);
            s.output = format!("spawn error: {e}");
            return;
        }
    };

    let stdout = child.stdout.take().unwrap();
    let stderr = child.stderr.take().unwrap();

    let status_out = status.clone();
    tokio::spawn(async move {
        let mut reader = BufReader::new(stdout).lines();
        let mut buf = String::new();
        while let Ok(Some(line)) = reader.next_line().await {
            buf.push_str(&line);
            buf.push('\n');
            if buf.len() > 10240 { buf = buf[buf.len().saturating_sub(10240)..].to_string(); }
            let mut s = status_out.lock().unwrap();
            s.output = buf.clone();
        }
    });

    let status_err = status.clone();
    tokio::spawn(async move {
        let mut reader = BufReader::new(stderr).lines();
        while let Ok(Some(line)) = reader.next_line().await {
            let mut s = status_err.lock().unwrap();
            s.output.push_str(&line);
            s.output.push('\n');
            if s.output.len() > 10240 { s.output = s.output[s.output.len().saturating_sub(10240)..].to_string(); }
        }
    });

    let timeout = tokio::time::sleep(Duration::from_secs(timeout_seconds));
    tokio::pin!(timeout);
    let exit_status = tokio::select! {
        r = child.wait() => r,
        _ = &mut timeout => { let _ = child.start_kill(); child.wait().await }
    };

    let mut s = status.lock().unwrap();
    s.running = false;
    s.exit_code = exit_status.ok().and_then(|e| e.code());
}
