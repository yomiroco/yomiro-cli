SHELL := /bin/bash
VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X github.com/yomiroco/yomiro-cli/internal/buildinfo.Version=$(VERSION) \
           -X github.com/yomiroco/yomiro-cli/internal/buildinfo.Commit=$(COMMIT) \
           -X github.com/yomiroco/yomiro-cli/internal/buildinfo.Date=$(DATE)

OAPI_CODEGEN_VERSION ?= v2.6.0

.PHONY: build test lint tidy oapi-codegen

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o ./bin/yomiro ./cmd/yomiro

test:
	go test ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

oapi-codegen:
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
