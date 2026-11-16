SHELL := /bin/bash

ENV_FILE ?= $(firstword $(wildcard .env))
PROD_ENV_FILE ?= .env.server

define load_env
if [ -f "$(ENV_FILE)" ]; then \
	set -a; \
	while IFS= read -r line || [ -n "$$line" ]; do \
		case "$$line" in \#*|'') continue ;; esac; \
		if [[ "$$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$$ ]]; then \
			export "$${BASH_REMATCH[1]}=$${BASH_REMATCH[2]}"; \
		fi; \
	done < "$(ENV_FILE)"; \
	set +a; \
fi;
endef

FRONTEND_PORT ?= 3000

.PHONY: api-types api-types-check infra infra-down infra-logs infra-ps migrate-up backend-dev frontend-dev local-dev demo-seed perf-seed perf-load \
	e2e-up e2e-test e2e-down free-dev-ports prod-env-init prod-env-upgrade prod-env-check env-check compose-env-check prod-config prod-up prod-down host-up host-down prod-smoke tools create-super-admin search-reindex check-i18n

infra:
	@if [ -n "$(ENV_FILE)" ]; then \
		docker compose --env-file "$(ENV_FILE)" -f compose.local.yml up -d; \
	else \
		docker compose -f compose.local.yml up -d; \
	fi

infra-down:
	@if [ -n "$(ENV_FILE)" ]; then \
		docker compose --env-file "$(ENV_FILE)" -f compose.local.yml down; \
	else \
		docker compose -f compose.local.yml down; \
	fi

infra-logs:
	docker compose -f compose.local.yml logs -f

infra-ps:
	docker compose -f compose.local.yml ps

migrate-up:
	@$(load_env) $(MAKE) -C backend migrate-up

backend-dev:
	@$(load_env) $(MAKE) -C backend dev

frontend-dev:
	@$(load_env) \
	cd frontend && \
	if [ ! -d node_modules ]; then pnpm install; fi && \
	pnpm dev

# Host ports bound by air + Next.js. Leaves Docker-mapped infra ports alone.
free-dev-ports:
	@$(load_env) \
	free_listen_port() { \
		local port="$$1"; \
		if ! command -v lsof >/dev/null 2>&1; then \
			echo "lsof not found; skip :$$port"; \
			return 0; \
		fi; \
		local attempt pids pid comm ppid pcomm sig; \
		for attempt in 1 2; do \
			pids=$$(lsof -nP -iTCP:"$$port" -sTCP:LISTEN -t 2>/dev/null | sort -u); \
			if [ -z "$$pids" ]; then \
				if [ "$$attempt" -eq 1 ]; then echo ":$$port is free"; fi; \
				break; \
			fi; \
			for pid in $$pids; do \
				comm=$$(ps -o comm= -p "$$pid" 2>/dev/null | tr -d '[:space:]'); \
				case "$$comm" in \
					*docker*|*com.docker*|dockerd) echo "skipping docker on :$$port (pid $$pid)"; continue ;; \
				esac; \
				ppid=$$(ps -o ppid= -p "$$pid" 2>/dev/null | tr -d '[:space:]'); \
				pcomm=$$(ps -o comm= -p "$$ppid" 2>/dev/null | tr -d '[:space:]'); \
				sig="-TERM"; \
				if [ "$$attempt" -eq 2 ]; then sig="-KILL"; fi; \
				echo "freeing :$$port pid $$pid ($$comm) $$sig"; \
				case "$$pcomm" in \
					*air*|*next*|*node*|*pnpm*|*npm*) kill "$$sig" "$$ppid" 2>/dev/null || true ;; \
				esac; \
				kill "$$sig" "$$pid" 2>/dev/null || true; \
			done; \
			sleep 0.4; \
		done; \
	}; \
	http_addr="$${APP_HTTP_ADDR:-:8080}"; \
	backend_port="$${http_addr##*:}"; \
	frontend_port="$${PORT:-$(FRONTEND_PORT)}"; \
	echo "Checking host ports $$backend_port (api) and $$frontend_port (frontend)..."; \
	free_listen_port "$$backend_port"; \
	free_listen_port "$$frontend_port"

# Infra in Docker; backend (air) + frontend (pnpm) on the host. No app image builds.
local-dev: infra
	@echo "Waiting for postgres..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
		docker compose -f compose.local.yml exec -T postgres pg_isready -U $${DB_USER:-app} -d $${DB_NAME:-app} >/dev/null 2>&1 && break; \
		sleep 1; \
	done
	@$(load_env) $(MAKE) -C backend migrate-up || true
	@$(MAKE) free-dev-ports
	@echo "Starting backend (air) + frontend (pnpm) — Ctrl+C stops both"
	@trap 'kill 0' INT TERM; \
	$(MAKE) backend-dev & \
	$(MAKE) frontend-dev & \
	wait

# Production env from deploy/env/schema (docs/design/env-core.md).
# Extra flags: make prod-env-init ARGS="--set APP_URL=https://app.example.com --yes"
prod-env-init:
	scripts/prod-env.sh init --file $(PROD_ENV_FILE) $(ARGS)

prod-env-upgrade:
	scripts/prod-env.sh upgrade --file $(PROD_ENV_FILE) $(ARGS)

prod-env-check:
	scripts/prod-env.sh check --file $(PROD_ENV_FILE) $(ARGS)

# CI check: .env.server.example matches the schema, .env.example has every
# schema key, prod-env tests pass. Fix drift: scripts/prod-env.sh example > .env.server.example
env-check: compose-env-check
	scripts/check-env-sync.sh

# CI check: every env var the backend reads reaches the backend/worker
# containers in compose.prod.yml, or has a decision in deploy/env/compose-env.allow.
compose-env-check:
	scripts/check-compose-env.sh
	scripts/check-compose-env.test.sh

# API tipleri (#276): backend/docs/openapi.yaml değişince frontend, mobile ve
# mobile-office tiplerini birlikte üret. CI (api-types işi) main ile
# birleştirilmiş hâli api-types-check ile denetler.
api-types:
	scripts/api-types.sh

api-types-check:
	scripts/api-types.sh --check

prod-config:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml config

# Prebuilt images: CI publishes them to GHCR and the server pulls them (Dokploy
# deploys on v0.* tags). `up -d` pulls per pull_policy (default always); nothing
# is built on the host. To smoke-test a local build, `docker build` the images
# in deploy/images.json, point BACKEND_IMAGE/FRONTEND_IMAGE at them and set
# IMAGE_PULL_POLICY=missing.
prod-up:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml up -d

prod-down:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml down

# Plain Docker host (no Dokploy): app stack + Caddy reverse proxy (docs/deploy.md).
# Reads .env.server at the repo root.
host-up:
	docker compose -f deploy/compose.prod.yml up -d

host-down:
	docker compose -f deploy/compose.prod.yml down

# Empty-host smoke test of deploy/compose.prod.yml with locally built images
# local/hakan-backend:main, local/hakan-frontend:main and local/hakan-backup:main
# (CI: prod-smoke).
prod-smoke:
	scripts/prod-smoke.sh

tools:
	@echo "docker:  $$(docker version --format '{{.Server.Version}}' 2>&1)"
	@echo "compose: $$(docker compose version 2>&1)"
	@echo "env:     $(ENV_FILE)"
	@$(MAKE) -C backend tools

# Bootstrap platform super_admin (loads root .env). Override: SA_EMAIL SA_PASSWORD SA_NAME SA_SURNAME
SA_EMAIL    ?= admin@example.com
SA_PASSWORD ?= Password1
SA_NAME     ?= Platform
SA_SURNAME  ?= Admin

# Temiz veritabanında tek komutla demo (#17): göçler + demo ofisi.
# Baştan kurmak için: make demo-seed ARGS=-reset
demo-seed: migrate-up
	@$(load_env) $(MAKE) -C backend demo-seed ARGS="$(ARGS)"

# Yük testi verisi (#182): 500 mükellef, 10.000 fatura. Üretimde çalışmaz.
# Baştan kurmak için: make perf-seed ARGS=-reset
perf-seed: migrate-up
	@$(load_env) $(MAKE) -C backend perf-seed ARGS="$(ARGS)"

# k6 ile temel yük testi (docs/performans.md). k6 kurulu olmalı.
perf-load:
	k6 run scripts/perf/k6-invoices.js

create-super-admin:
	@$(load_env) $(MAKE) -C backend create-super-admin \
		SA_EMAIL="$(SA_EMAIL)" \
		SA_PASSWORD="$(SA_PASSWORD)" \
		SA_NAME="$(SA_NAME)" \
		SA_SURNAME="$(SA_SURNAME)"

search-reindex:
	@$(load_env) $(MAKE) -C backend search-reindex

check-i18n:
	node scripts/check-i18n.mjs

# Uçtan uca duman testi (#170, e2e/README.md). Ortam her koşudan önce
# sıfırdan kurulur: make e2e-up && make e2e-test; make e2e-down.
e2e-up:
	scripts/e2e-up.sh

e2e-test:
	cd e2e && npx playwright test

e2e-down:
	scripts/e2e-down.sh
