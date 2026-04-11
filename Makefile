.PHONY: build test cover lint clean install

build:
	go build -o ouroboros ./

test:
	go test ./...

cover:
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run

clean:
	rm -f ouroboros coverage.out

install:
	claude mcp add --scope user ouroboros -- $(PWD)/ouroboros
