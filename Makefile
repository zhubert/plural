.PHONY: build test clean generate

build: generate
	go clean -cache
	go build -o plural .

test:
	go test ./...

generate:
	go generate ./...

clean:
	go clean -cache
	rm -f plural
