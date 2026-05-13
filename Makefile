# Makefile for Mattermost Policy Plugin

PLUGIN_ID ?= com.archtis.mattermost-policy-plugin
PLUGIN_VERSION ?= $(shell node -p "require('./plugin.json').version")
BUNDLE_NAME ?= $(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz
INCLUDE_WEBAPP ?= false

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
ifeq ($(INCLUDE_WEBAPP),true)
bundle: build webapp
else
bundle: build
endif
	@echo "Creating plugin bundle..."
	rm -rf dist/bundle
	mkdir -p dist/bundle/server/dist
	cp dist/plugin-* dist/bundle/server/dist/
ifeq ($(INCLUDE_WEBAPP),true)
	mkdir -p dist/bundle/webapp/dist
	cp plugin.json dist/bundle/
	cp webapp/dist/main.js dist/bundle/webapp/dist/
else
	node -e "const fs=require('fs'); const manifest=require('./plugin.json'); delete manifest.webapp; fs.writeFileSync('dist/bundle/plugin.json', JSON.stringify(manifest, null, 2) + '\n');"
endif
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
	@echo "  bundle       - Create server-only distributable plugin bundle"
	@echo "  bundle INCLUDE_WEBAPP=true - Create bundle with internal webapp"
	@echo "  test         - Run tests"
	@echo "  verify       - Run Go tests and webapp build"
	@echo "  clean        - Remove build artifacts"
	@echo "  deps         - Install dependencies"
