.PHONY: build test clean

build:
	go build -o plural .

test:
	go test ./...

clean:
	go clean -cache
	rm -f plural
