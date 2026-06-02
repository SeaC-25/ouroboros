GOPROXY ?= https://goproxy.cn,direct
BIN_DIR ?= bin
BIN ?= $(BIN_DIR)/ouro
DIST_DIR ?= dist
VERSION ?= 0.1.0-dev

.PHONY: test build doctor doctor-strict release clean

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

clean:
	@rm -rf $(BIN_DIR) $(DIST_DIR)
