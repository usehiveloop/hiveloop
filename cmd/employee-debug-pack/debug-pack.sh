#!/usr/bin/env bash
set +e

debug_dir="${HIVY_DEBUG_OUT:-/tmp/employee-sandbox-debug-$(date -u +%Y%m%dT%H%M%SZ)}"
archive="${debug_dir}.tar.gz"
sandbox_id="${HIVY_DEBUG_SANDBOX_ID:-${DAYTONA_SANDBOX_ID:-unknown}}"
sensitive="${HIVY_DEBUG_SENSITIVE:-0}"

rm -rf "$debug_dir" "$archive"
mkdir -p "$debug_dir"/{system,processes,env,health,workspace,tooling,logs,data,tmp}

exec > >(tee "$debug_dir/RUN.log") 2>&1

redact_env() {
  awk -F= 'BEGIN{IGNORECASE=1}
    $1 ~ /(TOKEN|SECRET|PASSWORD|CREDENTIAL|AUTH|BEARER|PRIVATE|COOKIE)/ {print $1"=<redacted>"; next}
    $1 ~ /(^|_)KEY($|_)/ {print $1"=<redacted>"; next}
    {print}'
}

capture() {
  out="$1"
  shift
  {
    printf '$'
    printf ' %q' "$@"
    printf '\n'
    "$@"
    code=$?
    printf '\nexit=%s\n' "$code"
  } > "$debug_dir/$out" 2>&1
}

capture_sh() {
  out="$1"
  shift
  {
    printf '$ bash -lc %q\n' "$*"
    bash -lc "$*"
    code=$?
    printf '\nexit=%s\n' "$code"
  } > "$debug_dir/$out" 2>&1
}

copy_if_exists() {
  src="$1"
  dest="$2"
  if [ -e "$src" ]; then
    mkdir -p "$(dirname "$debug_dir/$dest")"
    cp -a "$src" "$debug_dir/$dest" 2>/dev/null || true
  fi
}

safe_tail_copy() {
  src="$1"
  label="$(printf '%s' "$src" | sed 's#^/##; s#[^A-Za-z0-9._-]#_#g')"
  mkdir -p "$debug_dir/logs/tails"
  tail -c 1048576 "$src" > "$debug_dir/logs/tails/$label.tail" 2>/dev/null || true
}

bridge_pid="$(pgrep -f '[e]mployee-bridge|[e]mployee-runtime' | head -n 1 || true)"
if [ -z "$bridge_pid" ]; then
  bridge_pid=1
fi

{
  echo "Employee sandbox debug pack"
  echo "generated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "sandbox_id=$sandbox_id"
  echo "debug_dir=$debug_dir"
  echo "debug_archive=$archive"
  echo "bridge_pid=$bridge_pid"
  echo "sensitive=$sensitive"
  echo
  echo "Read first:"
  echo "$debug_dir/SUMMARY.txt"
  echo "$debug_dir/HOTSPOTS.txt"
} > "$debug_dir/SUMMARY.txt"

{
  echo "Hotspots"
  echo
  echo "Missing critical env keys:"
  missing=0
  for key in RUNTIME_SECRET HIVY_PROXY_API_KEY AGENT_MODEL AGENT_BASE_URL AGENT_API_KEY_ENV EMPLOYEE_ID CLOUD_CONTROL_PLANE_URL BRIDGE_API_KEY UPLOAD_BEARER WORKSPACE_ROOT DB_PATH RUNTIME_BIND_ADDR HIVY_SANDBOX_ID HIVY_ORG_ID HIVY_EMPLOYEE_ID HIVY_DRIVE_UPLOAD_URL; do
    eval "value=\${$key:-}"
    if [ -z "$value" ]; then
      echo "- $key"
      missing=1
    fi
  done
  if [ "$missing" = "0" ]; then
    echo "- none"
  fi
  echo
  echo "Raw provider API keys should normally be absent:"
  for key in OPENROUTER_API_KEY OPENAI_API_KEY GROQ_API_KEY TOGETHER_API_KEY; do
    eval "value=\${$key:-}"
    if [ -n "$value" ]; then
      echo "- present: $key"
    else
      echo "- absent: $key"
    fi
  done
  echo
  echo "Check health/healthz.txt and health/readyz.txt for bridge health."
  echo "Check data/sqlite-* files and data/employee-bridge.db if DB_PATH exists."
  if [ "$sensitive" != "1" ]; then
    echo "Sensitive env values are redacted. Re-run with sensitive mode only when needed."
  fi
} > "$debug_dir/HOTSPOTS.txt"

capture system/date.txt date -u
capture system/uname.txt uname -a
copy_if_exists /etc/os-release system/os-release
copy_if_exists /etc/resolv.conf system/resolv.conf
copy_if_exists /etc/hosts system/hosts
capture_sh system/df.txt 'df -h; echo; df -i'
capture_sh system/free.txt 'free -m || true'
capture_sh system/mount.txt 'mount || true'
capture_sh system/limits.txt 'ulimit -a'
capture_sh system/ip.txt 'ip addr 2>/dev/null; echo; ip route 2>/dev/null'
capture_sh system/listeners.txt 'ss -lntup 2>/dev/null || netstat -lntup 2>/dev/null || lsof -i -P -n 2>/dev/null || true'

capture_sh processes/ps.txt 'ps auxww'
capture_sh processes/top.txt 'top -b -n1 2>/dev/null || true'
capture_sh processes/pgrep.txt 'pgrep -af "employee|bridge|node|python" || true'
copy_if_exists "/proc/$bridge_pid/status" processes/bridge-status.txt
copy_if_exists "/proc/$bridge_pid/limits" processes/bridge-limits.txt
tr '\0' ' ' < "/proc/$bridge_pid/cmdline" > "$debug_dir/processes/bridge-cmdline.txt" 2>/dev/null || true
ls -la "/proc/$bridge_pid/fd" > "$debug_dir/processes/bridge-fds.txt" 2>&1 || true
tr '\0' '\n' < "/proc/$bridge_pid/environ" | sort > "$debug_dir/env/process-env.raw.tmp" 2>/dev/null || true
redact_env < "$debug_dir/env/process-env.raw.tmp" > "$debug_dir/env/process-env.redacted" 2>/dev/null || true
cut -d= -f1 "$debug_dir/env/process-env.raw.tmp" > "$debug_dir/env/process-env.keys" 2>/dev/null || true
if [ "$sensitive" = "1" ]; then
  mv "$debug_dir/env/process-env.raw.tmp" "$debug_dir/env/process-env.raw"
else
  rm -f "$debug_dir/env/process-env.raw.tmp"
fi
env | sort > "$debug_dir/env/shell-env.raw.tmp" 2>/dev/null || true
redact_env < "$debug_dir/env/shell-env.raw.tmp" > "$debug_dir/env/shell-env.redacted" 2>/dev/null || true
if [ "$sensitive" = "1" ]; then
  mv "$debug_dir/env/shell-env.raw.tmp" "$debug_dir/env/shell-env.raw"
else
  rm -f "$debug_dir/env/shell-env.raw.tmp"
fi

runtime_addr="${RUNTIME_BIND_ADDR:-0.0.0.0:7080}"
runtime_port="${runtime_addr##*:}"
runtime_base="http://127.0.0.1:${runtime_port}"
{
  echo "$ runtime_base=$runtime_base"
  curl -fsS -i --max-time 10 "$runtime_base/healthz"
  code=$?
  echo
  echo "exit=$code"
} > "$debug_dir/health/healthz.txt" 2>&1
{
  echo "$ runtime_base=$runtime_base"
  bearer="${BRIDGE_API_KEY:-${RUNTIME_SECRET:-}}"
  if [ -n "$bearer" ]; then
    curl -fsS -i --max-time 10 -H "Authorization: Bearer ${bearer}" "$runtime_base/readyz"
  else
    echo "no BRIDGE_API_KEY or RUNTIME_SECRET available"
  fi
  code=$?
  echo
  echo "exit=$code"
} > "$debug_dir/health/readyz.txt" 2>&1
capture_sh health/control-plane-health.txt 'if [ -n "${CLOUD_CONTROL_PLANE_URL:-}" ]; then curl -fsS -i --max-time 10 "${CLOUD_CONTROL_PLANE_URL%/}/healthz"; else echo "CLOUD_CONTROL_PLANE_URL not set"; fi'
capture_sh health/proxy-reachability.txt 'if [ -n "${AGENT_BASE_URL:-}" ]; then curl -fsS -i --max-time 10 "${AGENT_BASE_URL%/}" || true; else echo "AGENT_BASE_URL not set"; fi'

capture_sh tooling/versions.txt 'for tool in bash sh node npm pnpm yarn python3 python pip3 pip go git gh jq curl tar gzip zip unzip sqlite3 psql; do printf "%s: " "$tool"; command -v "$tool" || true; "$tool" --version 2>/dev/null | head -n 2 || true; echo; done'

if [ -d /workspace ]; then
  capture_sh workspace/listing.txt 'ls -la /workspace; echo; find /workspace -maxdepth 3 -mindepth 1 -printf "%M %u %g %s %TY-%Tm-%Td %TH:%TM %p\n" 2>/dev/null | head -2000'
  capture_sh workspace/git-status.txt 'find /workspace -maxdepth 4 -type d -name .git 2>/dev/null | while read -r gitdir; do repo="$(dirname "$gitdir")"; echo "## $repo"; git -C "$repo" status --short --branch 2>&1; echo; done'
else
  echo "/workspace missing" > "$debug_dir/workspace/listing.txt"
fi

if [ -n "${DB_PATH:-}" ] && [ -f "$DB_PATH" ]; then
  cp -a "$DB_PATH" "$debug_dir/data/employee-bridge.db" 2>/dev/null || true
  if command -v sqlite3 >/dev/null 2>&1; then
    sqlite3 "$DB_PATH" '.tables' > "$debug_dir/data/sqlite-tables.txt" 2>&1
    sqlite3 "$DB_PATH" '.schema' > "$debug_dir/data/sqlite-schema.sql" 2>&1
    sqlite3 "$DB_PATH" "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;" > "$debug_dir/data/sqlite-table-names.txt" 2>&1
    {
      while read -r table; do
        [ -n "$table" ] || continue
        printf '%s=' "$table"
        sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM \"$table\";" 2>/dev/null || echo "error"
      done < "$debug_dir/data/sqlite-table-names.txt"
    } > "$debug_dir/data/sqlite-counts.txt" 2>&1
  else
    echo "sqlite3 not installed" > "$debug_dir/data/sqlite-missing.txt"
  fi
else
  echo "DB_PATH missing or not a file: ${DB_PATH:-}" > "$debug_dir/data/sqlite-missing.txt"
fi

{
  for dir in /app /tmp /var/log; do
    [ -d "$dir" ] || continue
    find "$dir" -maxdepth 4 -type f \( -name '*.log' -o -name '*.out' -o -name '*.err' \) -printf '%s %TY-%Tm-%Td %TH:%TM %p\n' 2>/dev/null
  done | sort -nr | head -200
} > "$debug_dir/logs/log-manifest.txt"
while read -r size date time file; do
  [ -n "$file" ] || continue
  safe_tail_copy "$file"
done < "$debug_dir/logs/log-manifest.txt"

{
  echo "debug_dir=$debug_dir"
  echo "debug_archive=$archive"
  echo "Read first: $debug_dir/SUMMARY.txt and $debug_dir/HOTSPOTS.txt"
} > "$debug_dir/README.txt"

tar -czf "$archive" -C "$(dirname "$debug_dir")" "$(basename "$debug_dir")"
tar_code=$?
echo "debug_dir=$debug_dir"
echo "debug_archive=$archive"
echo "tar_exit=$tar_code"
exit "$tar_code"
