CURRENT_DIR := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
BUILD_TIME  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LAST_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
OUTPUT_DIR_TEMP ?= "${PWD}/output_temp"

.PHONY: linux darwin clean proto_lint proto build prerequisites dependencies test

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cloudscan-orchestrator-amd64 -ldflags "-X main.commit=${LAST_COMMIT} -X main.buildDate=${BUILD_TIME}" ./cmd/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o cloudscan-orchestrator-arm64 -ldflags "-X main.commit=${LAST_COMMIT} -X main.buildDate=${BUILD_TIME}" ./cmd/main.go

darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o cloudscan-orchestrator-amd64 -ldflags "-X main.commit=${LAST_COMMIT} -X main.buildDate=${BUILD_TIME}" ./cmd/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o cloudscan-orchestrator-arm64 -ldflags "-X main.commit=${LAST_COMMIT} -X main.buildDate=${BUILD_TIME}" ./cmd/main.go

clean:
	rm -rf cloudscan-orchestrator-*
	rm -rf output_temp

proto_lint:
	clang-format -style=google -i ./protobuf/*.proto 2>/dev/null || true

proto: proto_lint
	mkdir -p generated/protobuf
	protoc -I protobuf --go_out=./generated/protobuf --go_opt=paths=source_relative --go-grpc_out=./generated/protobuf --go-grpc_opt=paths=source_relative ./protobuf/*.proto

prerequisites:
	@echo "Installing Go dependencies..."
	go mod download
	go mod verify

build: prerequisites linux
	@echo "Building orchestrator binaries complete"
	@ls -lh cloudscan-orchestrator-*

dependencies: prerequisites
	@echo "Dependencies installed"

test:
	go test -v ./...