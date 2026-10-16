# KafkaLens

[![ci](https://github.com/fededomm/kafkalens/actions/workflows/ci.yml/badge.svg)](https://github.com/fededomm/kafkalens/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/fededomm/kafkalens)](https://github.com/fededomm/kafkalens/releases/latest)
[![license](https://img.shields.io/github/license/fededomm/kafkalens)](LICENSE)

A fast, single-binary Apache Kafka GUI: Go backend ([franz-go](https://github.com/twmb/franz-go)) + Angular UI embedded in the binary.
Runs in your browser or as a desktop app (Linux, Windows, macOS).

## Download

Get the latest build from [Releases](https://github.com/fededomm/kafkalens/releases/latest) and check it against `SHA256SUMS.txt`. What changed in each version: [CHANGELOG.md](CHANGELOG.md).

| OS | File | Install |
|---|---|---|
| Linux | `kafkalens-linux-amd64.tar.gz` | `tar xzf kafkalens-linux-amd64.tar.gz && ./kafkalens` (needs `libwebkit2gtk-4.1`) |
| Windows | `kafkalens-windows-amd64.exe` | Run it (needs WebView2, preinstalled on Windows 10/11) |
| macOS | `KafkaLens-macos-universal.dmg` | Open it, drag KafkaLens to Applications |

The binaries are not code-signed yet. Windows SmartScreen: *More info → Run anyway*. macOS: *System Settings → Privacy & Security → Open Anyway*, or `xattr -cr /Applications/KafkaLens.app`.

## Features

- **Clusters** — multiple connections, PLAINTEXT/TLS (custom CA, mTLS), SASL PLAIN/SCRAM-256/512, connection test, color tag, **read-only mode** (blocks writes from the UI; a guard against mistakes, not access control — restrict the SASL user with broker ACLs)
- **Overview** — brokers, controller, partition leadership, under-replicated/offline partitions, broker configs
- **Topics** — search (regex), sort, message counts, health, ★ favorites pinned on top (per cluster, stored in the browser), create (partitions/RF/retention/cleanup), delete with typed confirmation
- **Messages** — browse newest/oldest/from offset/from time, per partition, server-side filter on key/value/headers, JSON pretty-print + highlighting, headers, copy, export JSON, **live tail** (SSE), re-send a message
- **Produce** — key/null key, value with JSON validation/format, headers, target partition, N copies; optional per-message templates (`{{uuid}}`, `{{i}}`, `{{seq "a" "b"}}`, …)
- **Topic admin** — partitions (leader/replicas/ISR/offsets), add partitions, edit/reset configs, consumers with lag
- **Consumer groups** — state, members, lag per topic/partition, auto-refresh, reset offsets (earliest/latest/time/offset), delete
- **Ctrl+K** command palette, dark/light theme, keyboard navigation in the message list

## Run

```sh
make build && ./bin/kafkalens          # opens http://127.0.0.1:8088
./bin/kafkalens -addr 0.0.0.0:8088 -no-open -config /path/clusters.json
./bin/kafkalens -settings /path/config.yaml
```

**Settings.** Startup settings live in [`config.yaml`](config.yaml): listen address, whether to open the browser, the clusters file and the log level. They are read with [viper](https://github.com/spf13/viper), each layer overriding the previous one: the built-in defaults (the embedded `config.yaml`), then `./config.yaml` or the file given with `-settings`, then `KAFKALENS_<SECTION>_<KEY>` environment variables (e.g. `KAFKALENS_SERVER_ADDR`, `KAFKALENS_LOG_LEVEL`), then the `-addr`, `-config` and `-no-open` flags. Keys left out keep their default and unknown keys are rejected. The desktop app uses the embedded file plus the environment: edit `config.yaml` and rebuild, or set the variables.

Connections are stored in `~/.config/kafkalens/clusters.json` (mode 0600, passwords and client keys in plain text).
KafkaLens is a development tool: the API has no authentication, so only bind it to non-loopback addresses on trusted networks.

**Logs.** The browser mode logs to the console. The desktop app writes `kafkalens.log` next to `clusters.json` (`~/.config/kafkalens/` on Linux, `~/Library/Application Support/kafkalens/` on macOS, `%AppData%\kafkalens\` on Windows); past 10 MiB the old file is kept as `kafkalens.log.1` at the next start. Each API call that fails or changes something is logged with its status, duration and error; set `log.level: debug` in `config.yaml` to log reads too (`info` is the default, `warn` and `error` are quieter). `KAFKALENS_LOG_LEVEL` overrides it, which is the way to change the level of an installed desktop app without rebuilding. Request bodies are never logged.

## Desktop app (Wails)

The same UI and API in a native window: no browser, no TCP port. Built with [Wails v2](https://wails.io) (`desktop.go`, build tag `desktop`); the browser mode above stays pure Go.

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
make desktop           # Linux:   build/bin/kafkalens      (needs libwebkit2gtk-4.1-dev, libgtk-3-dev)
make desktop-windows   # Windows: build/bin/kafkalens.exe  (cross-compiled; needs WebView2, preinstalled on Windows 10/11)
```

On a Mac (needs Xcode Command Line Tools: `xcode-select --install`, plus Go, Node and the wails CLI above):

```sh
make desktop-macos     # macOS:   build/bin/KafkaLens.app + KafkaLens.dmg (universal: Intel + Apple Silicon)
```

macOS can only be built on a Mac (Apple SDK). Without one, push to GitHub and run the `desktop` workflow (`.github/workflows/desktop.yml`), which builds Linux, Windows and a universal macOS installer (`KafkaLens.dmg`: open it and drag KafkaLens into Applications) on native runners. The app is unsigned: first launch with right-click → Open, or `xattr -cr /Applications/KafkaLens.app`.

Desktop differences: live tail uses Wails events instead of SSE (Wails' asset server buffers responses), and exports open a native save dialog.

## Develop

```sh
make kafka   # single-node Kafka (KRaft) on localhost:9092
make dev     # Go API on :8088, Angular dev server on :4200 with /api proxied
```

## Contributing

Issues and pull requests are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md), and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how the pieces fit together. Report security issues privately as described in [SECURITY.md](SECURITY.md).

## Not (yet) included

Schema Registry (Avro/Protobuf decoding), Kafka Connect, ACL management, broker config editing.

## License

[Apache License 2.0](LICENSE)
