BINARY  := gloss
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build test lint e2e dev clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/gloss

test:
	go test ./...

lint:
	golangci-lint run
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: files need formatting:"; echo "$$unformatted"; exit 1; \
	fi

e2e:
	@echo "e2e arrives with milestone 4 (Playwright harness)"; exit 1

dev: build
	./$(BINARY) .

clean:
	rm -f $(BINARY)
