BINARY := nat
PKG := .

# The dev nat the macOS app shells out to (its NAT_BIN). One path shared by
# every launch, so the app and the nat it runs are never out of step — a
# stale one fails with the previous build's refusals.
NAT_DEV := /tmp/nat-dev

.PHONY: build vet test lint check run dev clean

build:
	go build -o $(BINARY) $(PKG)

vet:
	go vet ./...

test:
	go test -race -cover ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed; skipping"; \
	fi

check: vet test lint

run: build
	./$(BINARY)

# Build the Go binary and the Swift app, then run the app against the
# fresh binary.
dev:
	go build -o $(NAT_DEV) $(PKG)
	swift build --package-path macos
	NAT_BIN=$(NAT_DEV) macos/.build/debug/gnat

clean:
	rm -f $(BINARY)
