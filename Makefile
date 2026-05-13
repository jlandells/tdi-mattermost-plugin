# Makefile for Mattermost Policy Plugin

PLUGIN_ID ?= com.archtis.mattermost-policy-plugin
PLUGIN_VERSION ?= 1.0.5
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

## Build the webapp
.PHONY: webapp
webapp:
	@echo "Building webapp..."
	cd webapp && npm ci && npm run build

## Bundle the plugin for distribution
## Mattermost expects plugin.json at the ROOT of the extracted archive (no top-level folder)
.PHONY: bundle
bundle: build webapp
	@echo "Creating plugin bundle..."
	rm -rf dist/bundle
	mkdir -p dist/bundle/server/dist dist/bundle/webapp/dist
	cp plugin.json dist/bundle/
	cp dist/plugin-* dist/bundle/server/dist/
	cp webapp/dist/main.js dist/bundle/webapp/dist/
	cd dist/bundle && tar -czf ../$(BUNDLE_NAME) .
	@echo "Plugin bundle created: dist/$(BUNDLE_NAME)"

## Run tests
.PHONY: test
test:
	go test -v ./...

## Run production readiness verification
.PHONY: verify
verify:
	./scripts/verify.sh

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
	@echo "  verify       - Run Go tests and webapp build"
	@echo "  clean        - Remove build artifacts"
	@echo "  deps         - Install dependencies"
