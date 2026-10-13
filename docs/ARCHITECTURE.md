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
        mux["ServeMux<br/>/api routes + SPA fallback"]
        routes["api.go<br/>handlers · read-only guard<br/>tail: openTail / pumpTail (max 16)"]
        kafka["kafka.go<br/>clientOpts (TLS · SASL)<br/>Pool (one client per cluster) · Browse"]
        store["store.go<br/>Store · withSecrets"]
    end

    httpsrv --> harden
    assets --> harden
    harden --> mux --> routes
    routes --> kafka
    routes --> store
    bridge --> routes
    bridge --> store

    kafka -->|"franz-go kgo / kadm<br/>PLAINTEXT · TLS · SASL PLAIN/SCRAM"| clusters[("Kafka clusters")]
    store -->|"0600 file · atomic write"| file[("clusters.json<br/>user config dir")]
```

| Piece | Role |
|---|---|
| `web/` | Angular UI. Talks only to `/api/…`; in desktop mode it also uses the Wails bridge for live tail and native save dialogs. Theme and favorite topics live in `localStorage`. |
| `server.go` | Browser mode: listens on `127.0.0.1:8088` (`-addr`), opens the default browser. Pure Go, no cgo. |
| `desktop.go` | Desktop mode: serves the same handler through Wails' in-process asset server. Wails buffers whole responses, so live tail goes through Wails events and exports through `SaveFile`. |
| `main.go` | `newHandler()` wires the API and the embedded UI; `harden()` adds CSP and other security headers and caps request bodies. |
| `api.go` | REST routes. `withAdmin` resolves the cluster's admin client; `write` blocks mutations on read-only clusters; `openTail`/`pumpTail` stream new records for both modes. |
| `kafka.go` | franz-go client options (TLS, SASL), a pool with one long-lived client per cluster, and `Browse`, which reads a bounded window of records without joining a consumer group. |
| `store.go` | Saved connections in `clusters.json`. Secrets are never sent to the UI; `withSecrets` reuses a stored password only when brokers and auth settings are unchanged. |

## Request flow: reading messages

```mermaid
sequenceDiagram
    participant UI as Angular UI
    participant API as api.go
    participant ST as store.go
    participant K as kafka.go
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
