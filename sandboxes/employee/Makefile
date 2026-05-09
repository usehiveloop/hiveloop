.PHONY: build run dev test clean push-config

RUNTIME_SECRET ?= local-dev-runtime-token/Zkwn6QkmVIzHtDBWZCS81xXGpuGY172j1659w=
CONFIG_API := http://localhost:7080

build:
	cargo build

run: build
	@echo "=== Building and starting employee-bridge ==="
	@pgrep -f employee-bridge > /dev/null && pkill -f employee-bridge && sleep 1 || true
	@: > /tmp/employee-bridge.log
	@./target/debug/employee-bridge > /tmp/employee-bridge.log 2>&1 &
	@sleep 10
	@echo "=== Pushing config ==="
	@$(MAKE) push-config
	@echo ""
	@echo "=== Done. Bot PID: $$(pgrep -f employee-bridge | head -1) ==="
	@grep -c "error\|panic" /tmp/employee-bridge.log > /dev/null && echo "ISSUES IN LOGS" || echo "Logs clean"

push-config:
	@for i in 1 2 3 4 5; do \
		curl -s $(CONFIG_API)/config -H "Authorization: Bearer $(RUNTIME_SECRET)" > /tmp/current_config.json 2>/dev/null && break; \
		sleep 2; \
	done
	@python3 scripts/apply_config.py < /tmp/current_config.json > /tmp/updated_config.json
	@curl -s -X PUT $(CONFIG_API)/config -H "Authorization: Bearer $(RUNTIME_SECRET)" \
		-H "Content-Type: application/json" -d @/tmp/updated_config.json | \
		python3 -c "import json,sys; d=json.load(sys.stdin); print('Config applied:', d['applied_at'])"

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
	@sqlite3 data/employee-bridge.db "DELETE FROM agent_config;" 2>/dev/null || true
	@echo "Cleaned"
