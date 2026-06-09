DEV_ADDR ?= 127.0.0.1:8787
DEV_PORT ?= 8787
DEV_DIR ?= .tmp
DEV_BIN ?= ./codex-history
DEV_PID ?= $(DEV_DIR)/codex-history.pid
DEV_LOG ?= $(DEV_DIR)/codex-history.log

.PHONY: build test index serve dev_build dev_start dev_stop dev_restart dev_status

build:
	go build -o ./codex-history ./cmd/codex-history

test:
	go test ./...

index:
	go run ./cmd/codex-history index

serve:
	go run ./cmd/codex-history serve

dev_build:
	go build -o $(DEV_BIN) ./cmd/codex-history

dev_start: dev_build
	@mkdir -p $(DEV_DIR)
	@python3 -c 'import pathlib, subprocess; log=open("$(DEV_LOG)", "ab"); p=subprocess.Popen(["$(abspath $(DEV_BIN))", "serve", "--addr", "$(DEV_ADDR)"], cwd="$(CURDIR)", stdin=subprocess.DEVNULL, stdout=log, stderr=subprocess.STDOUT, start_new_session=True); pathlib.Path("$(DEV_PID)").write_text(str(p.pid) + "\n"); print(">>> started codex-history pid=%s" % p.pid); print(">>> url: http://$(DEV_ADDR)"); print(">>> log: $(DEV_LOG)")'

dev_stop:
	@echo ">>> stopping codex-history dev server"
	@if [ -f "$(DEV_PID)" ]; then \
		pid="$$(cat "$(DEV_PID)" 2>/dev/null || true)"; \
		if [ -n "$$pid" ] && kill -0 "$$pid" 2>/dev/null; then \
			echo ">>> kill pid $$pid"; \
			kill "$$pid" 2>/dev/null || true; \
		fi; \
		rm -f "$(DEV_PID)"; \
	fi
	@if command -v lsof >/dev/null 2>&1; then \
		pids="$$(lsof -tiTCP:$(DEV_PORT) -sTCP:LISTEN 2>/dev/null || true)"; \
		if [ -n "$$pids" ]; then \
			echo ">>> kill listeners on port $(DEV_PORT): $$pids"; \
			kill $$pids 2>/dev/null || true; \
		fi; \
	fi
	@sleep 0.5
	@if command -v lsof >/dev/null 2>&1; then \
		pids="$$(lsof -tiTCP:$(DEV_PORT) -sTCP:LISTEN 2>/dev/null || true)"; \
		if [ -n "$$pids" ]; then \
			echo ">>> force kill listeners on port $(DEV_PORT): $$pids"; \
			kill -9 $$pids 2>/dev/null || true; \
		fi; \
	fi

dev_restart: dev_stop dev_start

dev_status:
	@if [ -f "$(DEV_PID)" ]; then \
		pid="$$(cat "$(DEV_PID)" 2>/dev/null || true)"; \
		if [ -n "$$pid" ] && kill -0 "$$pid" 2>/dev/null; then \
			echo "pid: $$pid"; \
		else \
			echo "pid file exists but process is not running"; \
		fi; \
	else \
		echo "pid: none"; \
	fi
	@if command -v lsof >/dev/null 2>&1; then \
		lsof -nP -iTCP:$(DEV_PORT) -sTCP:LISTEN 2>/dev/null || true; \
	fi
	@echo "log: $(DEV_LOG)"
