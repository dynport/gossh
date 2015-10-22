all: build vet test

build:
	go get ./...

vet:
	go vet ./...

test:
	go test ./...
