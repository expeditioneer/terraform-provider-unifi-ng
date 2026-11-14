# terraform-provider-unifi
# Spec source: https://beez.ly/unifi-apis/

BINARY_NAME = terraform-provider-unifi

.PHONY: build clean fetch help

## build: Compile the provider binary
build:
	@echo "Building $(BINARY_NAME)..."
	go mod tidy
	go build -o $(BINARY_NAME) .
	@echo "Built $(BINARY_NAME)"

## clean: Remove the binary
clean:
	rm -f $(BINARY_NAME)

## fetch: Download the latest UniFi OpenAPI spec as API reference
fetch:
	@bash scripts/fetch-spec.sh

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
