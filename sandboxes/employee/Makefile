.PHONY: build run dev test clean push-config

RUNTIME_SECRET ?= local-dev-runtime-token/Zkwn6QkmVIzHtDBWZCS81xXGpuGY172j1659w=
CONFIG_API := http://localhost:7080
CONFIG_FILE := ./config.json

build:
	cargo build

run: build
	@echo "=== Building and starting employee-bridge ==="
	@pgrep -f employee-bridge > /dev/null && pkill -f employee-bridge && sleep 1 || true
	@: > /tmp/employee-bridge.log
	@./target/debug/employee-bridge > /tmp/employee-bridge.log 2>&1 &
	@sleep 10
	@echo "=== Pushing config from $(CONFIG_FILE) ==="
	@$(MAKE) push-config
	@echo ""
	@echo "=== Done. Bot PID: $$(pgrep -f employee-bridge | head -1) ==="

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
	          --test model_helpers_integration --test handler_routing_integration

clean:
	cargo clean
	@pgrep -f employee-bridge > /dev/null && pkill -f employee-bridge || true
	@rm -rf data
	@echo "Cleaned"
