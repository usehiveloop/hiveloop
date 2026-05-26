.PHONY: build run dev test clean push-config openapi

HIVY_RUNTIME_SECRET ?= local-dev-runtime-token
CONFIG_API := http://localhost:7080
CONFIG_FILE := ./config.json

build:
	cargo build

run: build
	@echo "=== Building and starting hivy-sandboxes-runtime ==="
	@pgrep -f hivy-sandboxes-runtime > /dev/null && pkill -f hivy-sandboxes-runtime && sleep 1 || true
	@: > /tmp/hivy-sandboxes-runtime.log
	@./target/debug/hivy-sandboxes-runtime > /tmp/hivy-sandboxes-runtime.log 2>&1 &
	@sleep 10
	@echo "=== Pushing config from $(CONFIG_FILE) ==="
	@$(MAKE) push-config CONFIG_FILE=$(CONFIG_FILE)
	@echo ""
	@echo "=== Done. Bot PID: $$(pgrep -f hivy-sandboxes-runtime | head -1) ==="

push-config:
	@for i in 1 2 3 4 5; do \
		curl -s -X PUT $(CONFIG_API)/config \
			-H "Authorization: Bearer $(HIVY_RUNTIME_SECRET)" \
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

openapi:
	cargo run -p hivy-sandboxes-runtime --features openapi --bin gen-openapi -- openapi.json

clean:
	cargo clean
	@pgrep -f hivy-sandboxes-runtime > /dev/null && pkill -f hivy-sandboxes-runtime || true
	@rm -rf data
	@echo "Cleaned"
