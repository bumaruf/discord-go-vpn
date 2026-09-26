#   make           client for this system
#   make dist      Linux and Windows (amd64 and arm64), with SHA256SUMS
#   make icon      regenerate build/icon.ico from the panel favicon (panel/app/icon.svg)
#   make syso      regenerate the Windows resources (manifest, version info, icon)
#   make test      gofmt, go vet and the test suites
#   make lint      golangci-lint on Linux and Windows

BIN     := discord-go-vpn
PKG     := ./cmd/discord-go-vpn
DIST    := dist

# Reproducible build: unsigned binaries are verified by hash (README.md).
GOFLAGS := -trimpath
LDFLAGS := -buildid=

.PHONY: all dist icon syso lint test clean provisiond

all:
	@echo "· $(BIN) (this system)"
	@go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

dist:
	@mkdir -p $(DIST)
	@echo "· linux/amd64"
	@GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)           $(PKG)
	@echo "· windows/amd64"
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN).exe       $(PKG)
	@echo "· windows/arm64"
	@GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-arm64.exe $(PKG)
	@cd $(DIST) && sha256sum $(BIN) $(BIN).exe $(BIN)-arm64.exe > SHA256SUMS
	@ls -lh $(DIST)
	@echo
	@echo "publish SHA256SUMS with the binaries -- it is how anyone"
	@echo "checks they got what you built:"
	@cat $(DIST)/SHA256SUMS

# Needs ImageMagick; build/icon.ico is committed. The panel's favicon is the only source of the
# icon, rendered once at 1024px and scaled down to each size.
ICON_SVG := panel/app/icon.svg

icon:
	@convert -background none -density 2304 $(ICON_SVG) -resize 1024x1024 -write mpr:icon +delete \
	  \( mpr:icon -filter Lanczos -resize 16x16 \) \
	  \( mpr:icon -filter Lanczos -resize 24x24 \) \
	  \( mpr:icon -filter Lanczos -resize 32x32 \) \
	  \( mpr:icon -filter Lanczos -resize 48x48 \) \
	  \( mpr:icon -filter Lanczos -resize 64x64 \) \
	  \( mpr:icon -filter Lanczos -resize 128x128 \) \
	  \( mpr:icon -filter Lanczos -resize 256x256 \) \
	  build/icon.ico
	@echo "· build/icon.ico"

# The .syso files are committed: without them a build would lack the elevation manifest, the
# version info antivirus heuristics look for, and the icon.
syso:
	@go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.7.0 \
	  -64 -manifest build/app.manifest -icon build/icon.ico \
	  -o cmd/$(BIN)/rsrc_windows_amd64.syso build/versioninfo.json
	@go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.7.0 \
	  -64 -arm -manifest build/app.manifest -icon build/icon.ico \
	  -o cmd/$(BIN)/rsrc_windows_arm64.syso build/versioninfo.json

GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2

# go run builds the tool for $GOOS, so the Windows target reaches only the linter, through -exec.
lint:
	GOOS= GOARCH= go run $(GOLANGCI_LINT) run ./...
	GOOS= GOARCH= go run -exec 'env GOOS=windows GOARCH=amd64' $(GOLANGCI_LINT) run ./...

provisiond:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(DIST)/provisiond ./cmd/provisiond

# ./panel is skipped like in go.mod: its node_modules ships third-party .go files.
test:
	@unformatted="$$(find . -name '*.go' -not -path './.claude/worktrees/*' -not -path './panel/*' -exec gofmt -l {} +)" || exit 1; \
	if [ -n "$$unformatted" ]; then echo "gofmt: unformatted files:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	GOOS=windows GOARCH=amd64 go vet ./...
	go test ./... -count=1
	sh server/provisiond-up_test.sh
	sh server/wg-up_test.sh
	sh server/compose_test.sh
	sh server/deploy_test.sh

clean:
	rm -f $(BIN) $(BIN).exe $(BIN)-arm64.exe
	rm -rf $(DIST)
