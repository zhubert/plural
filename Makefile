.PHONY: build test clean demo

build:
	go build -o plural .

test:
	go test ./...

clean:
	go clean -cache
	rm -f plural

# Generate a demo GIF: make demo SCENARIO=basic
# Available scenarios: basic, comprehensive
SCENARIO ?= basic
DEMO_DIR := docs/demos
demo: build
	@mkdir -p $(DEMO_DIR)
	./plural demo cast $(SCENARIO) -o $(DEMO_DIR)/$(SCENARIO).cast
	agg $(DEMO_DIR)/$(SCENARIO).cast $(DEMO_DIR)/$(SCENARIO).gif --cols 120 --rows 40 --line-height 1.2 --font-family "MonaspiceAr Nerd Font Mono"
	@echo "Generated $(DEMO_DIR)/$(SCENARIO).gif"
