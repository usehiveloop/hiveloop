#!/usr/bin/env bash
set -euo pipefail

# hivy-guardian — process supervisor for preview dev servers.
# Starts a command, monitors its health, and auto-restarts on crash
# with exponential backoff.
#
# Usage:
#   hivy-guardian "<command>" --port <port> [--health-path <path>] [--max-retries <n>]
#
# Examples:
#   hivy-guardian "npx vite --port 5173 --host" --port 5173 --health-path /
#   hivy-guardian "bundle exec rails server -p 3000 -b 0.0.0.0" --port 3000 --health-path /up
#   hivy-guardian "npx next dev --port 3000" --port 3000 --health-path /

HEALTH_PORT=""
HEALTH_PATH="/"
MAX_RETRIES=10
HEALTH_INTERVAL=5
STABLE_RESET_SECONDS=30
HEALTH_FAIL_THRESHOLD=3

usage() {
    echo "Usage: hivy-guardian \"<command>\" --port <port> [options]"
    echo ""
    echo "Options:"
    echo "  --port <port>            Port to health-check (required)"
    echo "  --health-path <path>     HTTP path for health check (default: /)"
    echo "  --max-retries <n>        Max consecutive restart attempts (default: 10)"
    echo "  --health-interval <s>    Seconds between health checks (default: 5)"
    echo "  -h, --help               Show this help"
    exit 2
}

log() {
    local ts
    ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "[guardian] ${ts} $*" >&2
}

COMMAND=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --port)
            HEALTH_PORT="$2"; shift 2 ;;
        --health-path)
            HEALTH_PATH="$2"; shift 2 ;;
        --max-retries)
            MAX_RETRIES="$2"; shift 2 ;;
        --health-interval)
            HEALTH_INTERVAL="$2"; shift 2 ;;
        -h|--help)
            usage ;;
        --)
            shift; COMMAND="$*"; break ;;
        -*)
            echo "Unknown option: $1" >&2; usage ;;
        *)
            if [[ -z "$COMMAND" ]]; then
                COMMAND="$1"
            else
                COMMAND="$COMMAND $1"
            fi
            shift ;;
    esac
done

if [[ -z "$COMMAND" ]]; then
    echo "Error: command is required" >&2
    usage
fi
if [[ -z "$HEALTH_PORT" ]]; then
    echo "Error: --port is required" >&2
    usage
fi

CHILD_PID=0
RESTART_COUNT=0
HEALTH_FAIL_COUNT=0
LAST_HEALTHY_TIME=0

cleanup() {
    if [[ $CHILD_PID -ne 0 ]]; then
        kill -TERM "$CHILD_PID" 2>/dev/null || true
        wait "$CHILD_PID" 2>/dev/null || true
    fi
    exit 0
}
trap cleanup SIGTERM SIGINT

start_child() {
    eval "$COMMAND" &
    CHILD_PID=$!
    log "started process pid=${CHILD_PID} command=\"${COMMAND}\""
}

check_health() {
    if ! nc -z -w2 localhost "$HEALTH_PORT" 2>/dev/null; then
        return 1
    fi
    if [[ "$HEALTH_PATH" != "" ]]; then
        if ! curl -sf -o /dev/null -m 5 "http://localhost:${HEALTH_PORT}${HEALTH_PATH}" 2>/dev/null; then
            return 1
        fi
    fi
    return 0
}

backoff_seconds() {
    local attempt=$1
    local delay=$((1 << attempt))
    if [[ $delay -gt 30 ]]; then
        delay=30
    fi
    echo "$delay"
}

start_child

while true; do
    sleep "$HEALTH_INTERVAL" &
    SLEEP_PID=$!
    wait $SLEEP_PID 2>/dev/null || true

    if ! kill -0 "$CHILD_PID" 2>/dev/null; then
        wait "$CHILD_PID" 2>/dev/null || true
        EXIT_CODE=$?
        log "process exited code=${EXIT_CODE}"

        RESTART_COUNT=$((RESTART_COUNT + 1))
        if [[ $RESTART_COUNT -gt $MAX_RETRIES ]]; then
            log "max retries (${MAX_RETRIES}) exceeded, giving up"
            exit 1
        fi

        BACKOFF=$(backoff_seconds "$RESTART_COUNT")
        log "restarting in ${BACKOFF}s (attempt ${RESTART_COUNT}/${MAX_RETRIES})"
        sleep "$BACKOFF"
        start_child
        HEALTH_FAIL_COUNT=0
        LAST_HEALTHY_TIME=$(date +%s)
        continue
    fi

    if check_health; then
        HEALTH_FAIL_COUNT=0
        local_now=$(date +%s)
        if [[ $LAST_HEALTHY_TIME -eq 0 ]]; then
            LAST_HEALTHY_TIME=$local_now
        fi
        stable_duration=$((local_now - LAST_HEALTHY_TIME))
        if [[ $stable_duration -ge $STABLE_RESET_SECONDS && $RESTART_COUNT -gt 0 ]]; then
            log "stable for ${stable_duration}s, resetting restart counter"
            RESTART_COUNT=0
        fi
    else
        HEALTH_FAIL_COUNT=$((HEALTH_FAIL_COUNT + 1))
        log "health check failed (consecutive: ${HEALTH_FAIL_COUNT})"
        if [[ $HEALTH_FAIL_COUNT -ge $HEALTH_FAIL_THRESHOLD ]]; then
            log "health fail threshold reached, killing process"
            kill -TERM "$CHILD_PID" 2>/dev/null || true
            wait "$CHILD_PID" 2>/dev/null || true

            RESTART_COUNT=$((RESTART_COUNT + 1))
            if [[ $RESTART_COUNT -gt $MAX_RETRIES ]]; then
                log "max retries (${MAX_RETRIES}) exceeded, giving up"
                exit 1
            fi

            BACKOFF=$(backoff_seconds "$RESTART_COUNT")
            log "restarting in ${BACKOFF}s (attempt ${RESTART_COUNT}/${MAX_RETRIES})"
            sleep "$BACKOFF"
            start_child
            HEALTH_FAIL_COUNT=0
            LAST_HEALTHY_TIME=$(date +%s)
        fi
    fi
done
