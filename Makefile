.PHONY: build test acc cover bench lint clean install

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-X dangernoodle.io/ouroboros/internal/cli.Version=$(VERSION)" -o ouroboros ./

test:
	go test ./...

acc:
	ACC_OUROBOROS=1 go test -timeout=120s ./integration/...

cover:
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

bench:
	go test ./internal/app -run TestResponseShapesFootprint -v

lint:
	golangci-lint run

clean:
	rm -f ouroboros coverage.out

install:
	claude mcp add --scope user ouroboros -- $(PWD)/ouroboros
