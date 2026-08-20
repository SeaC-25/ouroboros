GOPROXY ?= https://goproxy.cn,direct
BIN_DIR ?= bin
BIN ?= $(BIN_DIR)/ouro
DIST_DIR ?= dist
VERSION ?= 0.1.0-dev

.PHONY: test build doctor doctor-strict smoke-ide smoke-desktop-gui smoke-desktop-package smoke-workflow smoke-direct-use smoke-ai-provider smoke-workflow-closure smoke-release-managed smoke-managed-payloads smoke-production-acceptance smoke-final-evidence-fixture audit-completion release clean

test:
	@GOPROXY=$(GOPROXY) go test ./...

build:
	@GOPROXY=$(GOPROXY) BIN_DIR=$(BIN_DIR) BIN=$(BIN) ./scripts/build.sh

release:
	@GOPROXY=$(GOPROXY) DIST_DIR=$(DIST_DIR) VERSION=$(VERSION) ./scripts/release.sh

doctor:
	@workspace="$${WORKSPACE:-$$(mktemp -d /tmp/ouro-doctor-XXXXXX)/workspace}"; \
	GOPROXY=$(GOPROXY) go run ./cmd/ouro init "$$workspace" >/dev/null; \
	GOPROXY=$(GOPROXY) go run ./cmd/ouro doctor "$$workspace"

doctor-strict:
	@workspace="$${WORKSPACE:-$$(mktemp -d /tmp/ouro-doctor-strict-XXXXXX)/workspace}"; \
	GOPROXY=$(GOPROXY) go run ./cmd/ouro init "$$workspace" >/dev/null; \
	GOPROXY=$(GOPROXY) go run ./cmd/ouro doctor "$$workspace" --require-safety-authorization

smoke-ide:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_ide.sh

smoke-desktop-gui:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_desktop_gui.sh

smoke-desktop-package:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_desktop_package.sh

smoke-workflow:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_workflow.sh

smoke-direct-use:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_direct_use.sh

smoke-ai-provider:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_ai_provider.sh

smoke-workflow-closure:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_workflow_closure.sh

smoke-release-managed:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_release_managed.sh

smoke-managed-payloads:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_managed_payloads.sh

smoke-production-acceptance:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_production_acceptance.sh

smoke-final-evidence-fixture:
	@GOPROXY=$(GOPROXY) ./scripts/smoke_final_evidence_fixture.sh

audit-completion:
	@./scripts/audit_completion.sh

clean:
	@rm -rf $(BIN_DIR) $(DIST_DIR)
