.PHONY: all
all: deps lint test

GOPATH := $(shell go env GOPATH)

.PHONY: deps
deps:
	go mod download
	go install golang.org/x/tools/cmd/goimports@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: fmt
fmt:
	$(GOPATH)/bin/goimports -w .
	go fmt ./...

.PHONY: lint
lint:
	$(GOPATH)/bin/golangci-lint run ./...
	$(GOPATH)/bin/staticcheck ./...
	go vet ./...

.PHONY: test
test:
	go test -v -race ./...

.PHONY: cover
cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

.PHONY: clean
clean:
	go clean
	rm -f coverage.out

.PHONY: check
check: fmt lint test

.PHONY: build
build:
	go build -v ./...

.PHONY: run-proxy
run-proxy:
	go run ./cmd/hubproxy

.PHONY: run-testserver
run-testserver:
	go run ./cmd/dev/testserver

.PHONY: run-simulate
run-simulate:
	go run ./cmd/dev/simulate
