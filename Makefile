.PHONY: build web run dev kafka clean desktop desktop-windows desktop-macos

build: web            ## single binary with the UI embedded
	go build -trimpath -ldflags="-s -w" -o bin/kafkalens .

web:
	cd web && npm ci --silent && npx ng build

run: build
	./bin/kafkalens

dev:                  ## backend on :8088 + ng serve on :4200 (proxied)
	go run . -no-open & cd web && npx ng serve

kafka:                ## local Kafka on localhost:9092
	docker compose up -d

# Desktop app (Wails): same UI and API in a native window, no TCP port.
# macOS must be built on a Mac: see .github/workflows/desktop.yml.
WAILS ?= $(shell go env GOPATH)/bin/wails
WAILS_FLAGS = -trimpath -skipbindings

desktop:              ## Linux desktop binary in build/bin (needs libwebkit2gtk-4.1-dev)
	$(WAILS) build -platform linux/amd64 -tags webkit2_41 $(WAILS_FLAGS)

desktop-windows:      ## Windows .exe cross-compiled from Linux (needs WebView2, preinstalled on Win 10/11)
	$(WAILS) build -platform windows/amd64 $(WAILS_FLAGS)

desktop-macos:        ## on a Mac: universal KafkaLens.app + drag-to-Applications KafkaLens.dmg in build/bin
	@[ "$$(uname)" = Darwin ] || { echo "desktop-macos needs macOS (Apple SDK); use the GitHub workflow instead"; exit 1; }
	$(WAILS) build -platform darwin/universal $(WAILS_FLAGS)
	rm -rf build/dmg && mkdir -p build/dmg
	cp -R build/bin/KafkaLens.app build/dmg/
	ln -s /Applications build/dmg/Applications
	hdiutil create -volname KafkaLens -srcfolder build/dmg -format UDZO -ov build/bin/KafkaLens.dmg
	rm -rf build/dmg

clean:
	rm -rf bin web/dist build/bin
