# Contributing to KafkaLens

Thanks for helping! Bug reports, fixes and features are all welcome.

## Before you start

- **Bugs:** open an issue with the bug report form; include KafkaLens version, OS, mode (browser or desktop) and Kafka version.
- **Features:** open an issue first for anything bigger than a small fix, so we can agree on the approach before you write it.
- **Security issues:** don't open a public issue, see [SECURITY.md](SECURITY.md).

## Development setup

Requirements: Go (version in `go.mod`), Node 24, Docker (for a local Kafka). For the desktop app also the [Wails CLI](https://wails.io) and, on Linux, `libwebkit2gtk-4.1-dev libgtk-3-dev`.

```sh
make kafka   # 3-broker Kafka on localhost:9092,9094,9096
make dev     # Go API on :8088 + Angular dev server on :4200 with hot reload
make build   # browser-mode binary in bin/
make desktop # desktop app in build/bin/ (Linux)
```

Layout: `api.go` (REST routes), `kafka.go` (franz-go client, browse), `store.go` (saved clusters), `server.go` / `desktop.go` (browser and Wails entry points), `web/` (Angular UI).

## Checks

CI runs these on every pull request; run them before pushing:

```sh
cd web && npx ng build && cd ..
go vet ./... && go test ./...   # TestBrowse runs against localhost:9092 when `make kafka` is up
```

## Pull requests

- Keep each PR focused on one change; explain the why in the description.
- Match the style around your change; `gofmt` for Go, Prettier settings in `web/.prettierrc` for the UI.
- Add or update a test when you change behaviour.
- Never commit credentials: `clusters.json`, PEM keys and `.env` files are git-ignored for a reason.

By contributing you agree that your work is licensed under the [Apache License 2.0](LICENSE).
