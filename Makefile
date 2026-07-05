.PHONY: build install clean

build:
	go build ./...

install:
	go build -o $(shell go env GOPATH)/bin/herald ./cmd/cli

clean:
	go clean ./...
