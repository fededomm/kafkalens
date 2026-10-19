# Architecture

KafkaLens is one Go program with an embedded Angular UI. The same core runs in two modes:
**browser mode** (a local HTTP server, `server.go`) and **desktop mode** (a native Wails window, `desktop.go`).
The build tag `desktop` picks the entry point; everything else is shared.

## Runtime components

```mermaid
flowchart TB
    subgraph UI["Angular UI (web/, embedded with go:embed)"]
        pages["Pages<br/>clusters · overview · topics · topic · groups"]
        apisvc["Api service (api.ts)<br/>fetch /api/…"]
        ls[("localStorage<br/>theme · favorites")]
        pages --> apisvc
        pages --> ls
    end

    subgraph browser["Browser mode (server.go)"]
        httpsrv["http.Server<br/>127.0.0.1:8088"]
    end

    subgraph desktop["Desktop mode (desktop.go, Wails)"]
        assets["Wails AssetServer<br/>in-process, no TCP port"]
        bridge["Desktop bindings<br/>Tail · StopTail · SaveFile"]
    end

    apisvc -->|"HTTP · live tail over SSE"| httpsrv
    apisvc -->|"in-process HTTP"| assets
    apisvc -.->|"window.go calls · Wails events"| bridge

    subgraph core["Shared Go core"]
        harden["harden() · main.go<br/>security headers · 16 MiB body cap"]
        mux["chi router · main.go<br/>/api mount + SPA fallback"]
        routes["internal/api<br/>handlers · withCluster · writable guard"]
        kafka["internal/kafka<br/>ClientOpts (TLS · SASL)<br/>Pool (one client per cluster) · Browse<br/>tail: OpenTail / PumpTail (max 16)"]
        store["internal/store<br/>Store · WithSecrets"]
    end

    httpsrv --> harden
    assets --> harden
    harden --> mux --> routes
    routes --> kafka
    routes --> store
    bridge --> kafka
    bridge --> store

    kafka -->|"franz-go kgo / kadm<br/>PLAINTEXT · TLS · SASL PLAIN/SCRAM"| clusters[("Kafka clusters")]
    store -->|"0600 file · atomic write"| file[("clusters.json<br/>user config dir")]
```

| Piece | Role |
|---|---|
| `web/` | Angular UI. Talks only to `/api/…`; in desktop mode it also uses the Wails bridge for live tail and native save dialogs. Theme and favorite topics live in `localStorage`. |
| `server.go` | Browser mode: listens on `127.0.0.1:8088` (`-addr`), opens the default browser. Pure Go, no cgo. |
| `desktop.go` | Desktop mode: serves the same handler through Wails' in-process asset server. Wails buffers whole responses, so live tail goes through Wails events and exports through `SaveFile`. |
| `config.yaml`, `internal/config` | Startup settings (address, open browser, clusters file, log level), read with viper. Layers: embedded `config.yaml` (defaults) → `./config.yaml` or `-settings` (browser mode only, checked on its own before merging) → `KAFKALENS_<SECTION>_<KEY>` env → flags (browser mode only). Unknown keys, wrong types and invalid values stop the start with a clear error. |
| `logging.go` | [zerolog](https://github.com/rs/zerolog) setup: human-readable lines (`ConsoleWriter`) on the console in both modes, colored only on a terminal, plus `kafkalens.log` next to `clusters.json` in desktop mode (with the date, no colors); level from `log.level` (or `KAFKALENS_LOG_LEVEL`). The standard `log` package (net/http server errors) is routed through it as warnings. `internal/api/log.go` logs one line per API call. |
| `main.go` | `newHandler()` builds the [chi](https://github.com/go-chi/chi) router: mounts the API under `/api` and serves the embedded UI with an SPA fallback; `harden()` adds CSP and other security headers and caps request bodies. |
| `internal/api` | REST routes (`api.go`), with handlers grouped by area: `clusters.go`, `overview.go`, `topics.go`, `messages.go` (browse, live tail over SSE, produce, `template.go`), `groups.go`. The `withCluster` middleware resolves `{id}` to the cluster and its admin client with a 30 s timeout; `writable` blocks mutations on read-only clusters. Handlers return `(value, error)` and `reply` writes the JSON. |
| `internal/kafka` | franz-go client options (TLS, SASL), a pool with one long-lived client per cluster, `Browse`, which reads a bounded window of records without joining a consumer group, and `OpenTail`/`PumpTail`, which stream new records for both modes. |
| `internal/store` | Saved connections in `clusters.json`. Secrets are never sent to the UI; `WithSecrets` reuses a stored password only when brokers and auth settings are unchanged. |

## Request flow: reading messages

```mermaid
sequenceDiagram
    participant UI as Angular UI
    participant API as internal/api
    participant ST as internal/store
    participant K as internal/kafka
    participant KC as Kafka

    UI->>API: POST /api/c/{id}/topics/{topic}/browse
    API->>ST: Get(id)
    ST-->>API: Cluster (with secrets)
    API->>K: Pool.Admin(cluster)
    API->>K: Browse(topic, from, limit, filter)
    K->>KC: ListStartOffsets / ListEndOffsets
    K->>KC: fetch from computed offsets (no consumer group)
    KC-->>K: records
    K-->>API: records · scanned · partial
    API-->>UI: JSON
```

## Build and release

```mermaid
flowchart LR
    src["Go sources + web/"] --> ng["ng build<br/>web/dist"]
    ng --> gobuild["go build<br/>make build"] --> bin["bin/kafkalens<br/>browser mode"]
    ng --> wails["wails build (tag desktop)<br/>make desktop · desktop-windows · desktop-macos"]
    wails --> lin["Linux binary"]
    wails --> win["kafkalens.exe"]
    wails --> mac["KafkaLens.app + .dmg"]

    subgraph gha["GitHub Actions"]
        ci["ci.yml · every PR / push to main<br/>UI build · gofmt · vet · test · govulncheck"]
        rel["desktop.yml · tag v*<br/>Linux · Windows · macOS runners"]
        pub["release job<br/>SHA256SUMS · notes from CHANGELOG.md"]
        rel --> pub
    end
    pub --> ghr[("GitHub Release")]
```
