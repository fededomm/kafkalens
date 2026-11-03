# Security policy

## Reporting a vulnerability

Please **do not open a public issue**. Use GitHub's private reporting instead: **Security → Report a vulnerability** on this repository. You'll get an answer as soon as possible; please allow time for a fix before disclosing.

Only the latest release is supported with security fixes.

## Security model

KafkaLens is a **development tool**. Keep these properties in mind:

- **Browser mode** (`kafkalens`) runs a local HTTP API **without authentication**. It listens on `127.0.0.1` by default: anything that can reach that port can manage the configured clusters. To stop DNS rebinding and cross-site POSTs from web pages in your own browser, it only accepts requests whose `Host` is `localhost` or a loopback IP, and blocks cross-origin unsafe requests (POST, PUT, DELETE). Access from the LAN via a hostname or LAN IP is not supported.
- **Desktop mode** (Wails app): the UI talks to the backend in-process. The only TCP port is the MCP one, `127.0.0.1:8089` by default, which serves `/mcp` and nothing else.
- **MCP** (`/mcp`, on by default) requires `Authorization: Bearer <token>`; the token is in `mcp-token` next to `clusters.json` (mode 0600). It only exposes read-only tools, with the same `Host`/cross-origin checks as browser mode. In browser mode the unauthenticated API shows the token (the sidebar needs it), so there the token adds nothing over the API itself. Message contents go to the AI client and its provider; treat them as data, as the server's instructions tell the model to. Turn MCP off for good with `mcp.enabled: false` (desktop app: `KAFKALENS_MCP_ENABLED=false`, since it reads only its embedded settings).
- **Saved credentials** (SASL passwords, TLS client keys) are stored in plain text in `clusters.json`, readable only by your user (mode 0600). They are never sent back to the UI.
- **Read-only mode** guards against mistakes in the UI; it is not access control. Restrict what KafkaLens can do with Kafka ACLs on the SASL user.

Reports about these documented properties are welcome as hardening ideas, not as vulnerabilities.
