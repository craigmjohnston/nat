BINARY := notion-agent-tracker
PKG := ./cmd/notion-agent-tracker

.PHONY: build vet test lint check run clean

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

clean:
	rm -f $(BINARY)
