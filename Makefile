SHELL := /bin/sh

GO ?= go
CACHE_DIR ?= $(CURDIR)/.cache
UNAME_S := $(shell uname -s)
export GOCACHE := $(CACHE_DIR)/go-build

.PHONY: fmt fmt-check smoke-sqlite-vec test vet bench ci clean

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

smoke-sqlite-vec:
	$(GO) test ./internal/index -run TestCheckSQLiteReadyPasses -count=1

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

bench:
	$(GO) test ./internal/index ./internal/analysis -run '^$$' -bench . -benchmem

ci: fmt-check smoke-sqlite-vec test vet

clean:
	rm -rf $(CACHE_DIR)
