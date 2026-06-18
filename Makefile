.PHONY: lint build

lint:
	go vet ./...

build:
	go build -o marcaje ./cmd/marcaje
	go build -o marcajes-api ./cmd/marcajes-api
