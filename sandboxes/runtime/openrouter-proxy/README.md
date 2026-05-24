# OpenRouter Debug Proxy

This folder contains the local diagnostics proxy used to capture every
OpenAI-compatible request from `hivy-sandboxes-runtime` to OpenRouter. It is a
pass-through proxy: it does not normalize or mutate requests.

## Run With The Bridge

```sh
make run
```

`make run` starts the proxy, starts `hivy-sandboxes-runtime`, generates
`openrouter-proxy/config.proxy.json` from `config.json`, and pushes that config
to the bridge.

To bypass the proxy and use `config.json` directly:

```sh
make run-direct
```

## Logs

Each proxied request is written as one JSON file in:

```text
openrouter-proxy/logs/
```

The log includes request method/path/headers/body and response
status/headers/body. Authentication-style headers are redacted in logs, but
prompt and model response bodies are intentionally captured for debugging.

The proxy process stdout/stderr goes to:

```text
openrouter-proxy/proxy.log
```

## Manual Proxy Run

```sh
python3 openrouter-proxy/proxy.py
```

Useful environment variables:

```sh
OPENROUTER_PROXY_LISTEN=127.0.0.1
OPENROUTER_PROXY_PORT=7081
OPENROUTER_PROXY_TARGET=https://openrouter.ai/api/v1
OPENROUTER_PROXY_LOG_DIR=./openrouter-proxy/logs
OPENROUTER_PROXY_BASE_URL=http://127.0.0.1:7081/api/v1
OPENROUTER_PROXY_UPSTREAM_MAX_ATTEMPTS=4
```
