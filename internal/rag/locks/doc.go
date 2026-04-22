// Package locks provides Redis-backed coordination primitives for the
// three-loop sync architecture — per-connection single-flight,
// fencing-token locks, and heartbeat-liveness checks.
//
// Ports the Redis lock helpers scattered across Onyx's background tasks
// (backend/onyx/background/celery/tasks/*/tasks.py — every task that calls
// redis_client.lock(...)).
package locks
