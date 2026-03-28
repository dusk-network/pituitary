SHELL := /bin/sh

GO ?= go
CACHE_DIR ?= $(CURDIR)/.cache
UNAME_S := $(shell uname -s)
export GOCACHE := $(CACHE_DIR)/go-build

.PHONY: fmt fmt-check docs-check smoke-sqlite-vec test test-race vet analyze bench ci clean

CGO_ENABLED ?= 1
export CGO_ENABLED

ifeq ($(UNAME_S),Darwin)
CGO_CFLAGS += -Wno-deprecated-declarations
export CGO_CFLAGS
endif

fmt:
	$(GO) fmt ./...

fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		printf 'unformatted files:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi

docs-check:
	python3 scripts/check-doc-links.py README.md docs

smoke-sqlite-vec:
	$(GO) test ./internal/index -run TestCheckSQLiteReadyPasses -count=1

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	@! grep -R --include='*.go' -n '"github.com/dusk-network/pituitary/extensions' internal/ >/dev/null || { \
		echo 'internal/ must not import extensions/'; \
		exit 1; \
	}
	$(GO) vet ./...

analyze:
	@command -v staticcheck >/dev/null 2>&1 || { \
		echo 'staticcheck not found; install with: go install honnef.co/go/tools/cmd/staticcheck@v0.7.0'; \
		exit 1; \
	}
	@command -v govulncheck >/dev/null 2>&1 || { \
		echo 'govulncheck not found; install with: go install golang.org/x/vuln/cmd/govulncheck@v1.1.4'; \
		exit 1; \
	}
	@go_bin="$$(command -v $(GO) 2>/dev/null || printf '%s\n' '$(GO)')"; \
		[ -x "$$go_bin" ] || { \
			echo "Go toolchain not found via GO=$(GO)"; \
			exit 1; \
		}; \
		go_dir="$$(dirname "$$go_bin")"; \
		PATH="$$go_dir:$$PATH" staticcheck ./...; \
		PATH="$$go_dir:$$PATH" govulncheck ./...

bench:
	$(GO) test ./internal/index ./internal/analysis -run '^$$' -bench . -benchmem

ci: fmt-check docs-check smoke-sqlite-vec test vet

clean:
	rm -rf $(CACHE_DIR)
