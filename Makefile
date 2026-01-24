.PHONY: build test clean demo

build:
	go build -o plural .

test:
	go test -count=1 ./...

clean:
	go clean -cache
	rm -f plural

SCENARIO ?= overview
demo: build
	./plural demo cast $(SCENARIO) -o $(SCENARIO).cast
	agg $(SCENARIO).cast $(SCENARIO).gif --cols 120 --rows 40 --line-height 1.2 --font-family "MonaspiceAr Nerd Font Mono"
	@echo "Generated $(SCENARIO).gif"
