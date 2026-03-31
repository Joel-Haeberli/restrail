BINARY := restrail
GO := go
VERSION ?= dev
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build clean vet test run discover release-binaries

build:
	$(GO) build -ldflags "-X main.Version=$(VERSION)" -o $(BINARY) .

clean:
	rm -f $(BINARY)
	rm -rf dist

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

run: build
	./$(BINARY) run

discover: build
	./$(BINARY) discover-profile

release-binaries:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; ARCH=$${platform#*/}; \
		EXT=""; [ "$$OS" = "windows" ] && EXT=".exe"; \
		echo "Building $$OS/$$ARCH..."; \
		GOOS=$$OS GOARCH=$$ARCH $(GO) build -ldflags "-X main.Version=$(VERSION)" \
			-o dist/$(BINARY)-$$OS-$$ARCH$$EXT .; \
	done

release:
	@echo "Enter version number (e.g., v1.0.0):"
	@read -p "Version: " VERSION && \
	git tag $$VERSION && \
	git push origin $$VERSION && \
	echo "Released version $$VERSION"