use std::path::Path;

/// Snapshot of the runtime environment, collected once per conversation
/// and refreshed periodically. Always injected into conversations so the
/// agent knows where it is, what OS it's on, and (when applicable) the
/// resource envelope it's running in.
#[derive(Debug, Clone)]
pub struct EnvironmentSnapshot {
    /// Absolute path the bridge process is rooted in. Every tool (`bash`,
    /// `Read`, `write`, `edit`, etc.) interprets relative paths against
    /// this directory. This is the single most important fact in the
    /// reminder — agents that guess the path end up writing to `/tmp` or
    /// an invented `/workspace`.
    pub workspace_dir: String,
    pub os: String,
    pub memory_limit_mb: Option<u64>,
    pub memory_used_mb: Option<u64>,
    pub cpu_cores: Option<f64>,
    pub load_avg_1m: f64,
    pub disk_total_gb: f64,
    pub disk_used_gb: f64,
    pub disk_available_gb: f64,
}

const DEV_BOX_TOOLS: &str = "\
Node.js (LTS via nvm), Go 1.24, Rust (stable), Python 3.12, \
PostgreSQL 16 (dormant — start with `pg_ctlcluster 16 main start`), \
Redis 7 (dormant — start with `redis-server --daemonize yes`), \
SQLite 3, chrome-devtools-axi 0.1.15 (port 9224), gh-axi, \
chrome-headless-shell, gcc/g++/make, ffmpeg, jq, yq, curl, git, tmux";

impl EnvironmentSnapshot {
    pub fn collect() -> Self {
        Self {
            workspace_dir: read_workspace_dir(),
            os: read_os_version(),
            memory_limit_mb: read_memory_limit_mb(),
            memory_used_mb: read_memory_used_mb(),
            cpu_cores: read_cpu_cores(),
            load_avg_1m: read_load_avg(),
            disk_total_gb: 0.0,
            disk_used_gb: 0.0,
            disk_available_gb: 0.0,
        }
        .with_disk_stats()
    }

    fn with_disk_stats(mut self) -> Self {
        if let Some((total, used, avail)) = read_disk_stats("/") {
            self.disk_total_gb = total;
            self.disk_used_gb = used;
            self.disk_available_gb = avail;
        }
        self
    }

    /// Short version — always-safe reminder: workspace directory + OS +
    /// resources. Skips the sandbox-specific pre-installed-tools block.
    pub fn format_reminder(&self) -> String {
        self.format_reminder_with_options(false)
    }

    /// Full reminder. When `include_sandbox_tools` is true, append the
    /// pre-installed-tools section that only makes sense inside the
    /// Daytona dev-box template. Otherwise emit just the universally
    /// useful parts (workspace, OS, resources).
    pub fn format_reminder_with_options(&self, include_sandbox_tools: bool) -> String {
        let mut out = String::with_capacity(1024);

        out.push_str("# Environment\n\n");
        out.push_str(&format!("Workspace directory: `{}`\n", self.workspace_dir));
        out.push_str(
            "All relative paths resolve against this directory. `pwd` in a bash call \
             returns it. When a tool asks for a `file_path`, either give an absolute path \
             or a path relative to the workspace directory.\n\n",
        );
        out.push_str(&format!("OS: {}\n\n", self.os));

        if include_sandbox_tools {
            out.push_str("## Pre-installed tools (from sandbox template)\n");
            out.push_str(DEV_BOX_TOOLS);
            out.push_str("\n\n");
            out.push_str(
                "This list reflects what was pre-installed in the sandbox template. \
                 It does not include anything you may have installed yourself during \
                 this session (e.g. via apt, npm, pip, cargo install, etc.). \
                 If you installed something earlier in this conversation, you already \
                 know it's available — it won't appear here.\n\n",
            );
        }

        out.push_str("## Resources\n");

        match (self.memory_used_mb, self.memory_limit_mb) {
            (Some(used), Some(limit)) if limit > 0 => {
                let pct = (used as f64 / limit as f64 * 100.0).round();
                out.push_str(&format!(
                    "- Memory: {:.1} GB / {:.1} GB ({:.0}% used)\n",
                    used as f64 / 1024.0,
                    limit as f64 / 1024.0,
                    pct
                ));
            }
            _ => {
                out.push_str("- Memory: unknown (cgroup info unavailable)\n");
            }
        }

        if let Some(cores) = self.cpu_cores {
            out.push_str(&format!(
                "- CPU: {:.0} core{} (load avg: {:.2})\n",
                cores,
                if cores > 1.0 { "s" } else { "" },
                self.load_avg_1m
            ));
        } else {
            out.push_str(&format!(
                "- CPU: unknown (load avg: {:.2})\n",
                self.load_avg_1m
            ));
        }

        if self.disk_total_gb > 0.0 {
            let pct = (self.disk_used_gb / self.disk_total_gb * 100.0).round();
            out.push_str(&format!(
                "- Disk: {:.1} GB / {:.1} GB ({:.0}% used, {:.1} GB free)\n",
                self.disk_used_gb, self.disk_total_gb, pct, self.disk_available_gb
            ));
        }

        out.push_str(
            "\nMemory and CPU values reflect actual sandbox limits (from cgroups), not the host machine. \
             Databases are dormant at boot — start only what you need.",
        );

        out
    }
}

// ---------------------------------------------------------------------------
// cgroup v2 / v1 readers with fallbacks
// ---------------------------------------------------------------------------

fn read_memory_limit_mb() -> Option<u64> {
    read_cgroup_u64("/sys/fs/cgroup/memory.max")
        .or_else(|| read_cgroup_u64("/sys/fs/cgroup/memory/memory.limit_in_bytes"))
        .and_then(|bytes| {
            if bytes >= u64::MAX / 2 {
                None // "max" / effectively unlimited — not useful
            } else {
                Some(bytes / (1024 * 1024))
            }
        })
}

fn read_memory_used_mb() -> Option<u64> {
    read_cgroup_u64("/sys/fs/cgroup/memory.current")
        .or_else(|| read_cgroup_u64("/sys/fs/cgroup/memory/memory.usage_in_bytes"))
        .map(|bytes| bytes / (1024 * 1024))
}

fn read_cpu_cores() -> Option<f64> {
    // cgroup v2: /sys/fs/cgroup/cpu.max contains "QUOTA PERIOD" e.g. "200000 100000" = 2 cores
    if let Ok(content) = std::fs::read_to_string("/sys/fs/cgroup/cpu.max") {
        let parts: Vec<&str> = content.split_whitespace().collect();
        if parts.len() == 2 && parts[0] != "max" {
            if let (Ok(quota), Ok(period)) = (parts[0].parse::<f64>(), parts[1].parse::<f64>()) {
                if period > 0.0 {
                    return Some(quota / period);
                }
            }
        }
    }
    // cgroup v1
    let quota = read_cgroup_u64("/sys/fs/cgroup/cpu/cpu.cfs_quota_us");
    let period = read_cgroup_u64("/sys/fs/cgroup/cpu/cpu.cfs_period_us");
    if let (Some(q), Some(p)) = (quota, period) {
        if q > 0 && p > 0 {
            return Some(q as f64 / p as f64);
        }
    }
    None
}

fn read_workspace_dir() -> String {
    std::env::current_dir()
        .ok()
        .and_then(|p| p.canonicalize().ok().or(Some(p)))
        .map(|p| p.display().to_string())
        .unwrap_or_else(|| "(unknown — current_dir failed)".to_string())
}

fn read_load_avg() -> f64 {
    std::fs::read_to_string("/proc/loadavg")
        .ok()
        .and_then(|s| s.split_whitespace().next().and_then(|v| v.parse().ok()))
        .unwrap_or(0.0)
}

fn read_os_version() -> String {
    if let Ok(content) = std::fs::read_to_string("/etc/os-release") {
        for line in content.lines() {
            if let Some(pretty) = line.strip_prefix("PRETTY_NAME=") {
                return pretty.trim_matches('"').to_string();
            }
        }
    }
    std::env::consts::OS.to_string()
}

fn read_disk_stats(path: &str) -> Option<(f64, f64, f64)> {
    let p = Path::new(path);
    #[cfg(unix)]
    {
        let _ = std::fs::metadata(p).ok()?;
        let stat = nix_statvfs(p)?;
        let total = stat.0;
        let available = stat.1;
        let used = total - available;
        Some((
            total as f64 / (1024.0 * 1024.0 * 1024.0),
            used as f64 / (1024.0 * 1024.0 * 1024.0),
            available as f64 / (1024.0 * 1024.0 * 1024.0),
        ))
    }
    #[cfg(not(unix))]
    {
        let _ = p;
        None
    }
}

#[cfg(unix)]
#[allow(clippy::unnecessary_cast)]
fn nix_statvfs(path: &Path) -> Option<(u64, u64)> {
    use std::ffi::CString;
    let c_path = CString::new(path.to_str()?).ok()?;
    let mut stat: libc::statvfs = unsafe { std::mem::zeroed() };
    let ret = unsafe { libc::statvfs(c_path.as_ptr(), &mut stat) };
    if ret != 0 {
        return None;
    }
    // Casts needed: statvfs fields are u32 on macOS, u64 on Linux.
    let block_size = stat.f_frsize as u64;
    let total = stat.f_blocks as u64 * block_size;
    let available = stat.f_bavail as u64 * block_size;
    Some((total, available))
}

fn read_cgroup_u64(path: &str) -> Option<u64> {
    std::fs::read_to_string(path)
        .ok()
        .and_then(|s| s.trim().parse().ok())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn snapshot_collect_does_not_panic() {
        let snap = EnvironmentSnapshot::collect();
        assert!(!snap.os.is_empty());
        assert!(!snap.workspace_dir.is_empty());
        assert!(snap.load_avg_1m >= 0.0);
    }

    #[test]
    fn format_reminder_contains_key_sections() {
        let snap = EnvironmentSnapshot::collect();
        let text = snap.format_reminder();
        assert!(text.contains("# Environment"));
        assert!(text.contains("Workspace directory:"));
        assert!(text.contains("## Resources"));
        // Pre-installed-tools block is gated on `include_sandbox_tools`.
        assert!(!text.contains("## Pre-installed tools"));
    }

    #[test]
    fn format_reminder_with_sandbox_tools_opts_in() {
        let snap = EnvironmentSnapshot::collect();
        let text = snap.format_reminder_with_options(true);
        assert!(text.contains("## Pre-installed tools"));
        assert!(text.contains("does not include anything you may have installed"));
    }

    #[test]
    fn format_reminder_with_known_resources() {
        let snap = EnvironmentSnapshot {
            workspace_dir: "/workspace/proj".to_string(),
            os: "Ubuntu 24.04".to_string(),
            memory_limit_mb: Some(4096),
            memory_used_mb: Some(1024),
            cpu_cores: Some(2.0),
            load_avg_1m: 0.45,
            disk_total_gb: 20.0,
            disk_used_gb: 6.0,
            disk_available_gb: 14.0,
        };
        let text = snap.format_reminder();
        assert!(
            text.contains("Workspace directory: `/workspace/proj`"),
            "workspace line: {text}"
        );
        assert!(text.contains("1.0 GB / 4.0 GB"), "memory line: {text}");
        assert!(text.contains("2 cores"), "cpu line: {text}");
        assert!(text.contains("6.0 GB / 20.0 GB"), "disk line: {text}");
    }

    #[test]
    fn format_reminder_handles_missing_cgroup() {
        let snap = EnvironmentSnapshot {
            workspace_dir: "/tmp/testws".to_string(),
            os: "macOS".to_string(),
            memory_limit_mb: None,
            memory_used_mb: None,
            cpu_cores: None,
            load_avg_1m: 0.0,
            disk_total_gb: 0.0,
            disk_used_gb: 0.0,
            disk_available_gb: 0.0,
        };
        let text = snap.format_reminder();
        assert!(text.contains("unknown (cgroup info unavailable)"), "{text}");
        assert!(text.contains("unknown (load avg"), "{text}");
    }
}
