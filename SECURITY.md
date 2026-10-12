# Security policy

## Reporting a vulnerability

Please **do not open a public issue**. Use GitHub's private reporting instead: **Security → Report a vulnerability** on this repository. You'll get an answer as soon as possible; please allow time for a fix before disclosing.

Only the latest release is supported with security fixes.

## Security model

KafkaLens is a **development tool**. Keep these properties in mind:

- **Browser mode** (`kafkalens`) runs a local HTTP API **without authentication**. It listens on `127.0.0.1` by default: anything that can reach that port can manage the configured clusters. Don't bind it to other interfaces (`-addr 0.0.0.0:...`) on untrusted networks.
- **Desktop mode** (Wails app) opens no TCP port; the UI talks to the backend in-process.
- **Saved credentials** (SASL passwords, TLS client keys) are stored in plain text in `clusters.json`, readable only by your user (mode 0600). They are never sent back to the UI.
- **Read-only mode** guards against mistakes in the UI; it is not access control. Restrict what KafkaLens can do with Kafka ACLs on the SASL user.

Reports about these documented properties are welcome as hardening ideas, not as vulnerabilities.
