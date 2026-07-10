.PHONY: build test acc cover bench lint clean install

build:
	go build -o ouroboros ./

test:
	go test ./...

acc:
	ACC_OUROBOROS=1 go test -timeout=120s ./integration/...

cover:
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

bench: build
	python3 bench/response-shapes/probe.py

lint:
	golangci-lint run

clean:
	rm -f ouroboros coverage.out

install:
	claude mcp add --scope user ouroboros -- $(PWD)/ouroboros
