# Makefile for Mattermost Policy Plugin

PLUGIN_ID ?= com.archtis.mattermost-policy-plugin
PLUGIN_VERSION ?= 1.0.0
BUNDLE_NAME ?= $(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz

## Build the plugin for all supported platforms
.PHONY: build
build: build-linux build-darwin build-windows

.PHONY: build-linux
build-linux:
	@echo "Building for Linux..."
	GOOS=linux GOARCH=amd64 go build -o dist/plugin-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -o dist/plugin-linux-arm64 .

.PHONY: build-darwin
build-darwin:
	@echo "Building for macOS..."
	GOOS=darwin GOARCH=amd64 go build -o dist/plugin-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o dist/plugin-darwin-arm64 .

.PHONY: build-windows
build-windows:
	@echo "Building for Windows..."
	GOOS=windows GOARCH=amd64 go build -o dist/plugin-windows-amd64.exe .

## Bundle the plugin for distribution
.PHONY: bundle
bundle: build
	@echo "Creating plugin bundle..."
	mkdir -p dist/$(PLUGIN_ID)
	mkdir -p dist/$(PLUGIN_ID)/server/dist
	cp plugin.json dist/$(PLUGIN_ID)/
	cp dist/plugin-* dist/$(PLUGIN_ID)/server/dist/
	cd dist && tar -czf $(BUNDLE_NAME) $(PLUGIN_ID)
	@echo "Plugin bundle created: dist/$(BUNDLE_NAME)"

## Run tests
.PHONY: test
test:
	go test -v ./...

## Clean build artifacts
.PHONY: clean
clean:
	rm -rf dist/

## Install dependencies
.PHONY: deps
deps:
	go mod download
	go mod tidy

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build        - Build plugin for all platforms"
	@echo "  bundle       - Create distributable plugin bundle"
	@echo "  test         - Run tests"
	@echo "  clean        - Remove build artifacts"
	@echo "  deps         - Install dependencies"

