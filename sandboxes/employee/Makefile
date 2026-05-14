.PHONY: build run run-direct dev test clean push-config start-openrouter-proxy stop-openrouter-proxy proxy-config

RUNTIME_SECRET ?= local-dev-runtime-token/Zkwn6QkmVIzHtDBWZCS81xXGpuGY172j1659w=
CONFIG_API := http://localhost:7080
CONFIG_FILE := ./config.json
PROXY_CONFIG_FILE := ./openrouter-proxy/config.proxy.json
OPENROUTER_PROXY_LISTEN ?= 127.0.0.1
OPENROUTER_PROXY_PORT ?= 7081
OPENROUTER_PROXY_BASE_URL ?= http://$(OPENROUTER_PROXY_LISTEN):$(OPENROUTER_PROXY_PORT)/api/v1
OPENROUTER_PROXY_LOG_DIR ?= ./openrouter-proxy/logs
OPENROUTER_PROXY_PID := /tmp/employee-bridge-openrouter-proxy.pid

build:
	cargo build

run: build start-openrouter-proxy proxy-config
	@echo "=== Building and starting employee-bridge ==="
	@pgrep -f employee-bridge > /dev/null && pkill -f employee-bridge && sleep 1 || true
	@: > /tmp/employee-bridge.log
	@./target/debug/employee-bridge > /tmp/employee-bridge.log 2>&1 &
	@sleep 10
	@echo "=== Pushing config from $(PROXY_CONFIG_FILE) ==="
	@$(MAKE) push-config CONFIG_FILE=$(PROXY_CONFIG_FILE)
	@echo ""
	@echo "=== Done. Bot PID: $$(pgrep -f employee-bridge | head -1) ==="
	@echo "=== OpenRouter proxy logs: $(OPENROUTER_PROXY_LOG_DIR) ==="

run-direct: build
	@echo "=== Building and starting employee-bridge without OpenRouter proxy ==="
	@pgrep -f employee-bridge > /dev/null && pkill -f employee-bridge && sleep 1 || true
	@: > /tmp/employee-bridge.log
	@./target/debug/employee-bridge > /tmp/employee-bridge.log 2>&1 &
	@sleep 10
	@echo "=== Pushing config from $(CONFIG_FILE) ==="
	@$(MAKE) push-config CONFIG_FILE=$(CONFIG_FILE)
	@echo ""
	@echo "=== Done. Bot PID: $$(pgrep -f employee-bridge | head -1) ==="

start-openrouter-proxy:
	@mkdir -p $(OPENROUTER_PROXY_LOG_DIR)
	@if [ -f $(OPENROUTER_PROXY_PID) ] && kill -0 "$$(cat $(OPENROUTER_PROXY_PID))" 2>/dev/null; then \
		echo "=== OpenRouter proxy already running: $$(cat $(OPENROUTER_PROXY_PID)) ==="; \
	else \
		echo "=== Starting OpenRouter proxy on $(OPENROUTER_PROXY_BASE_URL) ==="; \
		OPENROUTER_PROXY_LISTEN=$(OPENROUTER_PROXY_LISTEN) \
		OPENROUTER_PROXY_PORT=$(OPENROUTER_PROXY_PORT) \
		OPENROUTER_PROXY_LOG_DIR=$(OPENROUTER_PROXY_LOG_DIR) \
		python3 ./openrouter-proxy/proxy.py > ./openrouter-proxy/proxy.log 2>&1 & \
		echo $$! > $(OPENROUTER_PROXY_PID); \
		sleep 1; \
	fi

stop-openrouter-proxy:
	@if [ -f $(OPENROUTER_PROXY_PID) ]; then \
		kill "$$(cat $(OPENROUTER_PROXY_PID))" 2>/dev/null || true; \
		rm -f $(OPENROUTER_PROXY_PID); \
	fi

proxy-config:
	@OPENROUTER_PROXY_BASE_URL=$(OPENROUTER_PROXY_BASE_URL) \
		python3 ./openrouter-proxy/config_for_proxy.py ./config.json > $(PROXY_CONFIG_FILE)

push-config:
	@for i in 1 2 3 4 5; do \
		curl -s -X PUT $(CONFIG_API)/config \
			-H "Authorization: Bearer $(RUNTIME_SECRET)" \
			-H "Content-Type: application/json" \
			-d @$(CONFIG_FILE) | \
			python3 -c "import json,sys; d=json.load(sys.stdin); print('Config applied:', d['applied_at'])"; \
		exit_code=$$?; \
		if [ $$exit_code -eq 0 ]; then break; fi; \
		sleep 2; \
	done

dev:
	cargo build && cargo watch -x run

test:
	cargo test --test cron_integration --test cron_tool_integration \
	          --test delegate_wake_integration --test scheduler_integration \
	          --test bash_operations_integration --test file_operations_integration \
	          --test process_registry_integration --test skills_integration \
	          --test handler_routing_integration

clean:
	cargo clean
	@pgrep -f employee-bridge > /dev/null && pkill -f employee-bridge || true
	@$(MAKE) stop-openrouter-proxy
	@rm -rf data
	@echo "Cleaned"
