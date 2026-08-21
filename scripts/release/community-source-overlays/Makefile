.DEFAULT_GOAL := help

.PHONY: test test-community test-enterprise lint build-all build-community build-enterprise build-gateway build-admin build-mcp build-sbs-exporter build-notification-adapter build-ops-report build-s3bench check-enterprise-build-source run-dev run-compat compat-user-space compat-public-s3 compat-sbs-physical-user-space compat-sbs-cluster-ec compat-awscli compat-mc compat-rclone compat-report container-packaging-check container-build container-local-up container-local-smoke container-local-down container-local-reset container-sbs-quickstart-up container-sbs-quickstart-smoke container-sbs-quickstart-down container-sbs-quickstart-reset container-community-up container-community-smoke container-community-failover-smoke container-community-down container-community-reset release-readiness production-scale-check release-artifact-metadata helm-chart-check community-source-check community-source-export community-release-check enterprise-release-check publication-readiness smoke-etcd-registry smoke-active-active smoke-metadata-backup-restore docs-source-check docs-build docs-render-check html-docs-check
.PHONY: k8s-production-values k8s-production-render k8s-production-deploy k8s-production-delete k8s-production-status kind-production-up kind-production-build-images kind-production-load-images kind-production-deploy kind-production-start kind-production-stop kind-production-down kind-production-delete
.PHONY: help build check-edition-boundary check-community-export export-community test-community-export check-publication-readiness smoke-sbs-session-refcount-open smoke-sbs-session-close-guard smoke-sbs-session-fence

GO ?= go
GOFLAGS_BASE ?= -buildvcs=false
GOFLAGS ?= $(GOFLAGS_BASE)
GOFLAGS_COMMUNITY ?= $(GOFLAGS_BASE)
GOFLAGS_ENTERPRISE ?= $(GOFLAGS_BASE) -tags=enterprise,namros_ec
VERSION_PACKAGE ?= github.com/nosway/namros/internal/version
NAMROS_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf dev)
NAMROS_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf unknown)
NAMROS_BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_LDFLAGS_VERSION := -X $(VERSION_PACKAGE).Version=$(NAMROS_VERSION) -X $(VERSION_PACKAGE).Commit=$(NAMROS_COMMIT) -X $(VERSION_PACKAGE).Date=$(NAMROS_BUILD_DATE)
GO_LDFLAGS ?= $(GO_LDFLAGS_VERSION)
GO_BUILD_FLAGS = $(GOFLAGS) -ldflags "$(GO_LDFLAGS)"
GO_BUILD_FLAGS_COMMUNITY = $(GOFLAGS_COMMUNITY) -ldflags "$(GO_LDFLAGS)"
GO_BUILD_FLAGS_ENTERPRISE = $(GOFLAGS_ENTERPRISE) -ldflags "$(GO_LDFLAGS)"
GOLANGCI_LINT ?= golangci-lint
MKDOCS ?= mkdocs
MKDOCS_CONFIG ?= mkdocs.yml
MKDOCS_SITE_DIR ?= site
DOCS_SOURCE_DIR ?= docs-src
DOCS_MANUAL_SOURCE_DIR ?= $(DOCS_SOURCE_DIR)/manuals
GREP ?= grep
BIN_DIR ?= $(CURDIR)/bin
COMMUNITY_BIN_DIR ?= $(BIN_DIR)/community
NAMROS_ENTERPRISE_REPO ?= $(CURDIR)
ENTERPRISE_BIN_DIR ?= $(NAMROS_ENTERPRISE_REPO)/bin/enterprise
CACHE_DIR ?= $(CURDIR)/.cache
GOCACHE ?= $(CACHE_DIR)/go-build
GOMODCACHE ?= $(CACHE_DIR)/gomod
ENTERPRISE_CACHE_DIR ?= $(NAMROS_ENTERPRISE_REPO)/.cache
ENTERPRISE_GOCACHE ?= $(ENTERPRISE_CACHE_DIR)/go-build
ENTERPRISE_GOMODCACHE ?= $(ENTERPRISE_CACHE_DIR)/gomod
NAMROS_CMDS := namros-gateway namros-admin namros-mcp namros-sbs-exporter namros-notification-adapter namros-ops-report namros-s3bench
COMMUNITY_CMDS ?= $(NAMROS_CMDS)
ENTERPRISE_CMDS ?= $(COMMUNITY_CMDS)
COMMUNITY_TEST_PACKAGES ?= ./...
ENTERPRISE_TEST_PACKAGES ?= ./...
COMMUNITY_GO_ENV = GOCACHE="$(GOCACHE)" GOMODCACHE="$(GOMODCACHE)"
ENTERPRISE_GO_ENV = GOCACHE="$(ENTERPRISE_GOCACHE)" GOMODCACHE="$(ENTERPRISE_GOMODCACHE)"
COMMUNITY_RELEASE_TARGETS ?= check-publication-readiness test-community docs-render-check check-community-export export-community
COMMUNITY_COMPAT_REPORT_TARGETS ?= compat-user-space smoke-etcd-registry smoke-active-active smoke-metadata-backup-restore s3fs-linux
NAMROS_COMPAT_AUTOSTART_GATEWAY ?= 1
DOCKER ?= docker
DOCKER_COMPOSE ?= $(DOCKER) compose
CONTAINER_COMPOSE_FILE ?= packaging/docker/compose.yaml
CONTAINER_COMMUNITY_COMPOSE_FILE ?= packaging/docker/compose.community.yml
CONTAINER_SBS_QUICKSTART_COMPOSE_FILE ?= packaging/docker/compose.sbs-quickstart.yml
CONTAINER_ENV_FILE ?= packaging/docker/.env
CONTAINER_PROFILE ?= local
K8S_PRODUCTION_CONFIG ?= packaging/k8s/production-kind.env
K8S_PRODUCTION_SCRIPT ?= scripts/k8s/deploy-production.sh
NAMROS_USE_NAMRBD_SBS_IMAGES_FILE := $(shell if [ -f "$(CONTAINER_ENV_FILE)" ]; then sed -n 's/^NAMROS_USE_NAMRBD_SBS_IMAGES=//p' "$(CONTAINER_ENV_FILE)" | tail -n 1; fi)
NAMROS_USE_NAMRBD_SBS_IMAGES ?= $(if $(NAMROS_USE_NAMRBD_SBS_IMAGES_FILE),$(NAMROS_USE_NAMRBD_SBS_IMAGES_FILE),0)
CONTAINER_SBS_BUILD_FLAG ?= $(if $(filter 1 true yes,$(NAMROS_USE_NAMRBD_SBS_IMAGES)),--no-build,--build)

help:
	@printf "Available targets:\n"
	@printf "  make build            Build all NAMROS binaries into %s\n" "$(BIN_DIR)"
	@printf "  make build-all        Alias for build\n"
	@printf "  make build-community  Build Community edition binaries into %s\n" "$(COMMUNITY_BIN_DIR)"
	@printf "  make test-community   Run Community edition tests and boundary checks\n"
	@printf "  make build-enterprise Build Enterprise edition binaries into %s\n" "$(ENTERPRISE_BIN_DIR)"
	@printf "  make test-enterprise  Run Enterprise edition tests and boundary checks\n"
	@printf "  make check-edition-boundary Check Community/Enterprise source boundaries\n"
	@printf "  make check-community-export Validate the Community source export boundary\n"
	@printf "  make export-community Create the Community source tree and tarball under dist/\n"
	@printf "  make test-community-export Export and self-check a public Community checkout\n"
	@printf "  make check-publication-readiness Run public repository readiness checks\n"
	@printf "  make production-scale-check Run local production-scale release gates\n"
	@printf "  make release-artifact-metadata Write release checksums, provenance, and dependency pins\n"
	@printf "  make helm-chart-check Validate the Community Helm chart contract\n"
	@printf "  make smoke-sbs-session-refcount-open Run the local SBS session refcount open smoke\n"
	@printf "  make smoke-sbs-session-close-guard Run the local SBS session close guard smoke\n"
	@printf "  make smoke-sbs-session-fence Run the local SBS stale session fence smoke\n"
	@printf "  make compat-user-space Run AWS CLI, MinIO client, and rclone compatibility smoke\n"
	@printf "  make compat-public-s3 Run the strict public aws-cli/mc/rclone S3 compatibility smoke\n"
	@printf "  make compat-report Create a Community compatibility matrix report\n"
	@printf "  make smoke-etcd-registry Run the etcd gateway registry smoke\n"
	@printf "  make smoke-active-active Run the active-active gateway smoke\n"
	@printf "  make smoke-metadata-backup-restore Run the metadata backup/restore smoke\n"
	@printf "  make release-readiness Create release-readiness JSON/Markdown artifacts\n"
	@printf "  make community-release-check Run the configured Community release target set\n"
	@printf "  make enterprise-release-check Run the configured Enterprise release target set\n"
	@printf "  make container-packaging-check Validate public container packaging metadata\n"
	@printf "  make container-local-smoke Run the local container S3 smoke\n"
	@printf "  make container-sbs-quickstart-smoke Run the gateway plus SBS backend quickstart smoke\n"
	@printf "  make container-community-smoke Run the Community cross-gateway and load-balancer smoke\n"
	@printf "  make k8s-production-render Render the production-shaped Kubernetes config from %s\n" "$(K8S_PRODUCTION_CONFIG)"
	@printf "  make kind-production-deploy Create a kind cluster and deploy the production-shaped topology\n"
	@printf "  make kind-production-start Deploy into the existing kind cluster without rebuilding images\n"
	@printf "  make kind-production-stop Uninstall NAMROS while keeping the kind cluster and images\n"
	@printf "  make kind-production-down Delete the production-shaped kind cluster and its test state\n"
	@printf "  make docs-render-check Build and verify the public documentation site\n"
	@printf "  make html-docs-check Alias for docs-render-check\n"

build: build-all

test:
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	$(COMMUNITY_GO_ENV) $(GO) test $(GOFLAGS) ./...

test-community: check-edition-boundary
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	$(COMMUNITY_GO_ENV) $(GO) test $(GOFLAGS_COMMUNITY) $(COMMUNITY_TEST_PACKAGES)

test-enterprise: check-enterprise-build-source
	mkdir -p "$(ENTERPRISE_GOCACHE)" "$(ENTERPRISE_GOMODCACHE)"
	cd "$(NAMROS_ENTERPRISE_REPO)" && NAMROS_ENTERPRISE_OVERLAY_TEST=1 $(ENTERPRISE_GO_ENV) $(GO) test $(GOFLAGS_ENTERPRISE) $(ENTERPRISE_TEST_PACKAGES)

lint:
	@if command -v $(GOLANGCI_LINT) >/dev/null 2>&1; then \
		$(GOLANGCI_LINT) run ./...; \
	else \
		$(GO) vet $(GOFLAGS) ./...; \
	fi

$(BIN_DIR) $(COMMUNITY_BIN_DIR) $(ENTERPRISE_BIN_DIR):
	mkdir -p "$@"

$(NAMROS_CMDS:%=$(BIN_DIR)/%): $(BIN_DIR)/%: | $(BIN_DIR)
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	$(COMMUNITY_GO_ENV) $(GO) build $(GO_BUILD_FLAGS) -o "$@" ./cmd/$*

$(COMMUNITY_CMDS:%=$(COMMUNITY_BIN_DIR)/%): $(COMMUNITY_BIN_DIR)/%: | $(COMMUNITY_BIN_DIR)
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	$(COMMUNITY_GO_ENV) $(GO) build $(GO_BUILD_FLAGS_COMMUNITY) -o "$@" ./cmd/$*

$(ENTERPRISE_CMDS:%=$(ENTERPRISE_BIN_DIR)/%): $(ENTERPRISE_BIN_DIR)/%: | $(ENTERPRISE_BIN_DIR)
	scripts/release/check-enterprise-build-source.sh "$(NAMROS_ENTERPRISE_REPO)"
	mkdir -p "$(ENTERPRISE_GOCACHE)" "$(ENTERPRISE_GOMODCACHE)"
	cd "$(NAMROS_ENTERPRISE_REPO)" && $(ENTERPRISE_GO_ENV) $(GO) build $(GO_BUILD_FLAGS_ENTERPRISE) -o "$@" ./cmd/$*

build-all: $(NAMROS_CMDS:%=$(BIN_DIR)/%)

build-community: check-edition-boundary $(COMMUNITY_CMDS:%=$(COMMUNITY_BIN_DIR)/%)

build-enterprise: check-enterprise-build-source $(ENTERPRISE_CMDS:%=$(ENTERPRISE_BIN_DIR)/%)

build-gateway: $(BIN_DIR)/namros-gateway

build-admin: $(BIN_DIR)/namros-admin

build-mcp: $(BIN_DIR)/namros-mcp

build-sbs-exporter: $(BIN_DIR)/namros-sbs-exporter

build-notification-adapter: $(BIN_DIR)/namros-notification-adapter

build-ops-report: $(BIN_DIR)/namros-ops-report

build-s3bench: $(BIN_DIR)/namros-s3bench

check-enterprise-build-source:
	scripts/release/check-enterprise-build-source.sh "$(NAMROS_ENTERPRISE_REPO)"

run-dev:
	$(GO) run ./cmd/namros-gateway \
		-listen 127.0.0.1:9000 \
		-region us-east-1 \
		-metadata-backend pebble \
		-metadata-path .namros/meta \
		-storage-backend local \
		-storage-path .namros/segments

run-compat:
	$(GO) run ./cmd/namros-gateway \
		-listen 0.0.0.0:9000 \
		-region us-east-1 \
		-metadata-backend pebble \
		-metadata-path .namros/meta \
		-storage-backend local \
		-storage-path .namros/segments

compat-user-space:
	NAMROS_COMPAT_AUTOSTART_GATEWAY=$(NAMROS_COMPAT_AUTOSTART_GATEWAY) scripts/compat/run-user-space-smoke.sh

compat-public-s3:
	NAMROS_COMPAT_AUTOSTART_GATEWAY=$(NAMROS_COMPAT_AUTOSTART_GATEWAY) scripts/compat/run-public-s3-compat-smoke.sh

compat-sbs-physical-user-space:
	scripts/compat/run-sbs-physical-user-space-smoke.sh

compat-sbs-cluster-ec:
	@printf 'compat-sbs-cluster-ec is an Enterprise/private-distribution target; the public Community source tree does not ship the EC smoke harness.\n' >&2
	@exit 1

compat-awscli:
	scripts/compat/awscli-smoke.sh

compat-mc:
	scripts/compat/mc-smoke.sh

compat-rclone:
	scripts/compat/rclone-smoke.sh

compat-report:
	NAMROS_COMPAT_REPORT_TARGETS="$(COMMUNITY_COMPAT_REPORT_TARGETS)" scripts/compat/run-matrix-report.sh

container-packaging-check:
	sh scripts/container/check-packaging.sh

container-build:
	sh scripts/container/ensure-local-files.sh
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMPOSE_FILE)" --profile "$(CONTAINER_PROFILE)" build

container-local-up:
	sh scripts/container/ensure-local-files.sh
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMPOSE_FILE)" --profile local up -d --build gateway

container-local-smoke:
	sh scripts/container/ensure-local-files.sh
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMPOSE_FILE)" --profile local up -d --build gateway
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMPOSE_FILE)" --profile local run --rm tools namros-container-local-smoke

container-local-down:
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMPOSE_FILE)" --profile local down

container-local-reset:
	@sh scripts/container/ensure-local-files.sh
	@project="$$(sed -n 's/^COMPOSE_PROJECT_NAME=//p' "$(CONTAINER_ENV_FILE)" | tail -n 1)"; \
		if [ -z "$$project" ]; then project=namros-community; fi; \
		printf '[container] removing Compose project and volumes: %s\n' "$$project"
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMPOSE_FILE)" --profile local down --volumes --remove-orphans

container-sbs-quickstart-up:
	sh scripts/container/ensure-local-files.sh
	@if [ "$(NAMROS_USE_NAMRBD_SBS_IMAGES)" != "1" ] && [ "$(NAMROS_USE_NAMRBD_SBS_IMAGES)" != "true" ] && [ "$(NAMROS_USE_NAMRBD_SBS_IMAGES)" != "yes" ]; then \
		$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_SBS_QUICKSTART_COMPOSE_FILE)" --profile sbs-quickstart build sbs-quickstart-service sbs-quickstart-data-1 sbs-quickstart-bootstrap; \
	fi
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_SBS_QUICKSTART_COMPOSE_FILE)" --profile sbs-quickstart build sbs-quickstart-pool-bootstrap sbs-quickstart-gateway
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_SBS_QUICKSTART_COMPOSE_FILE)" --profile sbs-quickstart up -d --no-build sbs-quickstart-pd sbs-quickstart-tikv sbs-quickstart-service sbs-quickstart-data-1 sbs-quickstart-data-2
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_SBS_QUICKSTART_COMPOSE_FILE)" --profile sbs-quickstart run --rm sbs-quickstart-bootstrap
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_SBS_QUICKSTART_COMPOSE_FILE)" --profile sbs-quickstart run --no-deps --rm sbs-quickstart-pool-bootstrap
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_SBS_QUICKSTART_COMPOSE_FILE)" --profile sbs-quickstart up -d --no-build sbs-quickstart-gateway

container-sbs-quickstart-smoke:
	$(MAKE) container-sbs-quickstart-up
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_SBS_QUICKSTART_COMPOSE_FILE)" --profile sbs-quickstart run --rm sbs-quickstart-tools namros-container-local-smoke

container-sbs-quickstart-down:
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_SBS_QUICKSTART_COMPOSE_FILE)" --profile sbs-quickstart down

container-sbs-quickstart-reset:
	@sh scripts/container/ensure-local-files.sh
	@project="$$(sed -n 's/^COMPOSE_PROJECT_NAME=//p' "$(CONTAINER_ENV_FILE)" | tail -n 1)"; \
		if [ -z "$$project" ]; then project=namros-community; fi; \
		printf '[container] removing SBS quickstart Compose project and volumes: %s\n' "$$project"
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_SBS_QUICKSTART_COMPOSE_FILE)" --profile sbs-quickstart down --volumes --remove-orphans

container-community-up:
	sh scripts/container/ensure-local-files.sh
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMMUNITY_COMPOSE_FILE)" --profile community up -d $(CONTAINER_SBS_BUILD_FLAG) etcd pd tikv sbs-service-1 sbs-service-2 sbs-data-1 sbs-data-2 sbs-data-3 sbs-data-4 sbs-service-lb sbs-data-lb
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMMUNITY_COMPOSE_FILE)" --profile community run $(CONTAINER_SBS_BUILD_FLAG) --rm sbs-bootstrap
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMMUNITY_COMPOSE_FILE)" --profile community run --build --rm namros-pool-bootstrap
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMMUNITY_COMPOSE_FILE)" --profile community up -d --build namros-gateway-a namros-gateway-b s3-lb

container-community-smoke:
	$(MAKE) container-community-up
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMMUNITY_COMPOSE_FILE)" --profile community run --rm community-tools namros-container-community-smoke

container-community-failover-smoke:
	$(MAKE) container-community-up
	CONTAINER_COMPOSE_FILE="$(CONTAINER_COMMUNITY_COMPOSE_FILE)" CONTAINER_ENV_FILE="$(CONTAINER_ENV_FILE)" DOCKER="$(DOCKER)" sh scripts/container/community-failover-smoke.sh

container-community-down:
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMMUNITY_COMPOSE_FILE)" --profile community down

container-community-reset:
	@sh scripts/container/ensure-local-files.sh
	@project="$$(sed -n 's/^COMPOSE_PROJECT_NAME=//p' "$(CONTAINER_ENV_FILE)" | tail -n 1)"; \
		if [ -z "$$project" ]; then project=namros-community; fi; \
		printf '[container] removing Compose project and volumes: %s\n' "$$project"
	$(DOCKER_COMPOSE) --env-file "$(CONTAINER_ENV_FILE)" -f "$(CONTAINER_COMMUNITY_COMPOSE_FILE)" --profile community down --volumes --remove-orphans

k8s-production-values:
	$(K8S_PRODUCTION_SCRIPT) write-values "$(K8S_PRODUCTION_CONFIG)"

k8s-production-render:
	$(K8S_PRODUCTION_SCRIPT) render "$(K8S_PRODUCTION_CONFIG)"

k8s-production-deploy:
	$(K8S_PRODUCTION_SCRIPT) deploy "$(K8S_PRODUCTION_CONFIG)"

k8s-production-delete:
	$(K8S_PRODUCTION_SCRIPT) delete "$(K8S_PRODUCTION_CONFIG)"

k8s-production-status:
	$(K8S_PRODUCTION_SCRIPT) status "$(K8S_PRODUCTION_CONFIG)"

kind-production-up:
	$(K8S_PRODUCTION_SCRIPT) kind-up "$(K8S_PRODUCTION_CONFIG)"

kind-production-build-images:
	$(K8S_PRODUCTION_SCRIPT) build-images "$(K8S_PRODUCTION_CONFIG)"

kind-production-load-images:
	$(K8S_PRODUCTION_SCRIPT) kind-load-images "$(K8S_PRODUCTION_CONFIG)"

kind-production-deploy:
	$(K8S_PRODUCTION_SCRIPT) kind-deploy "$(K8S_PRODUCTION_CONFIG)"

kind-production-start:
	$(K8S_PRODUCTION_SCRIPT) kind-start "$(K8S_PRODUCTION_CONFIG)"

kind-production-stop:
	$(K8S_PRODUCTION_SCRIPT) kind-stop "$(K8S_PRODUCTION_CONFIG)"

kind-production-down:
	$(K8S_PRODUCTION_SCRIPT) kind-down "$(K8S_PRODUCTION_CONFIG)"

# Backward-compatible alias.
kind-production-delete: kind-production-down

production-scale-check:
	scripts/release/check-production-scale-readiness.sh

release-artifact-metadata:
	scripts/release/write-release-artifact-metadata.sh

helm-chart-check:
	sh scripts/release/check-helm-chart.sh

smoke-sbs-session-refcount-open:
	scripts/lab/run-sbs-session-refcount-open-smoke.sh

smoke-sbs-session-close-guard:
	scripts/lab/run-sbs-session-close-guard-smoke.sh

smoke-sbs-session-fence:
	scripts/lab/run-sbs-session-fence-smoke.sh

check-edition-boundary: community-source-check

check-community-export: community-source-check

export-community: community-source-export

test-community-export: export-community
	NAMROS_COMMUNITY_CHECK_SKIP_GO_TEST=true make check-community-export

community-source-check:
	scripts/release/check-community-source.sh

community-source-export:
	scripts/release/export-community-source.sh

community-release-check:
	@for target in $(COMMUNITY_RELEASE_TARGETS); do \
		echo "[community-release] run $$target"; \
		$(MAKE) $$target; \
	done

publication-readiness:
	scripts/release/check-publication-readiness.sh

check-publication-readiness: publication-readiness

release-readiness:
	scripts/ops/run-release-readiness-report.sh

enterprise-release-check:
	@if [ "$(NAMROS_ENTERPRISE_REPO)" = "$(CURDIR)" ]; then \
		scripts/release/check-enterprise-build-source.sh "$(NAMROS_ENTERPRISE_REPO)"; \
	else \
		$(MAKE) -C "$(NAMROS_ENTERPRISE_REPO)" enterprise-release-check; \
	fi

smoke-etcd-registry:
	scripts/coordination/run-etcd-registry-smoke.sh

smoke-active-active:
	scripts/chaos/run-active-active-smoke.sh

smoke-metadata-backup-restore:
	scripts/metadata/run-backup-restore-smoke.sh

docs-source-check:
	test -f "$(MKDOCS_CONFIG)"
	test -f "$(DOCS_SOURCE_DIR)/index.md"
	test -f "$(DOCS_SOURCE_DIR)/requirements.txt"
	test -f "$(DOCS_SOURCE_DIR)/assets/namros-docs.css"
	test -f "$(DOCS_MANUAL_SOURCE_DIR)/index.md"
	test -f "$(DOCS_MANUAL_SOURCE_DIR)/installation-guide.md"
	test -f "$(DOCS_MANUAL_SOURCE_DIR)/user-manual.md"
	test -f "$(DOCS_MANUAL_SOURCE_DIR)/admin-guide.md"
	test -f "$(DOCS_MANUAL_SOURCE_DIR)/architecture-manual/index.md"
	test -f "$(DOCS_MANUAL_SOURCE_DIR)/ko/index.md"
	$(GREP) -Eq '^docs_dir:[[:space:]]+docs-src$$' "$(MKDOCS_CONFIG)"
	@# md_in_html renders Markdown inside the component <div> blocks the manual
	@# sources rely on. Without it the manual bodies publish as raw text.
	$(GREP) -Eq '^[[:space:]]+- md_in_html$$' "$(MKDOCS_CONFIG)"
	$(GREP) -Eq '^[[:space:]]+- assets/namros-docs\.css$$' "$(MKDOCS_CONFIG)"
	@# Page chrome belongs to the theme; embedded wrappers duplicate it.
	@if $(GREP) -rlE 'class="(layout|sidebar|topbar|chapter)"' "$(DOCS_SOURCE_DIR)" --include='*.md' >/dev/null 2>&1; then \
		printf 'embedded page-chrome wrapper found in docs-src; the theme owns layout\n' >&2; \
		$(GREP) -rlE 'class="(layout|sidebar|topbar|chapter)"' "$(DOCS_SOURCE_DIR)" --include='*.md' >&2; \
		exit 1; \
	fi
	@# mkdocs resolves .md sources, not published .html paths.
	@if $(GREP) -rnE '\]\([^)]*\.html' "$(DOCS_SOURCE_DIR)" --include='*.md' | $(GREP) -qvE '\]\(https?://'; then \
		printf 'internal .html link found in docs-src; link the .md source instead\n' >&2; \
		$(GREP) -rnE '\]\([^)]*\.html' "$(DOCS_SOURCE_DIR)" --include='*.md' | $(GREP) -vE '\]\(https?://' >&2; \
		exit 1; \
	fi
	bash scripts/docs/check-html-docs.sh

docs-build: docs-source-check
	$(MKDOCS) build --strict --config-file "$(MKDOCS_CONFIG)"

docs-render-check: docs-build
	@# `mkdocs build --strict` validates links and nav but not rendered output.
	@# Assert that no Markdown source markers survive into the published pages.
	@python3 tools/check-docs-render.py "$(MKDOCS_SITE_DIR)"

html-docs-check: docs-render-check
