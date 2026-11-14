# terraform-provider-unifi
# Spec source: https://beez.ly/unifi-apis/

BINARY_NAME = terraform-provider-unifi

.PHONY: build clean fetch help

## build: Compile the provider binary
build:
	go build -o $(BINARY_NAME) .

## clean: Remove the binary
clean:
	rm --force $(BINARY_NAME)

## fetch: Download the latest UniFi OpenAPI spec as API reference
fetch:
	@scripts/fetch-spec

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
