.PHONY: build test clean demo

build:
	go build -o plural .

test:
	go test -count=1 ./...

clean:
	go clean -cache
	rm -f plural

demo: build
	./plural demo cast overview -o overview.cast
	agg overview.cast demo.gif --cols 120 --rows 40 --line-height 1.2 --font-family "MonaspiceAr Nerd Font Mono"
	@echo "Generated demo.gif"
