GO ?= go
PROTOC ?= protoc

PROTO_FILES := $(wildcard api/*/v1/*.proto)
TOOLS_BIN := $(CURDIR)/.cache/bin
PROTOC_GEN_GO_VERSION := v1.36.12
PROTOC_GEN_GO_GRPC_VERSION := v1.6.2

.PHONY: build test tools generate

build:
	$(GO) build -trimpath -o bin/ ./cmd/...

test:
	$(GO) test ./...

tools:
	GOBIN="$(TOOLS_BIN)" $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	GOBIN="$(TOOLS_BIN)" $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)

generate: tools
	$(PROTOC) -I api --plugin=protoc-gen-go="$(TOOLS_BIN)/protoc-gen-go" --plugin=protoc-gen-go-grpc="$(TOOLS_BIN)/protoc-gen-go-grpc" --go_out=gen --go_opt=paths=source_relative --go-grpc_out=gen --go-grpc_opt=paths=source_relative $(PROTO_FILES)
