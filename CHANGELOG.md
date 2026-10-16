# Changelog

All notable changes to KafkaLens are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [0.3.0] - 2026-10-16

### Added

- **Message templates in Produce.** A new **Template** option renders the key, value and header values separately for each message, so sending N copies produces N different records. It uses Go `text/template` syntax with these functions: `{{uuid}}`, `{{i}}` (message index), `{{seq "a" "b"}}` (values in rotation), `{{pick "a" "b"}}` (random value), `{{randInt 1 100}}`, `{{now}}` (RFC 3339) and `{{unixMs}}`.
  While the option is on, the dialog lists the available functions and highlights template actions in the fields, marking invalid ones in red. JSON validation still works on templated values. Template errors are rejected before anything is sent, and the error names the field.

## [0.2.0] - 2026-10-13

### Added

- **Favorite topics.** Star a topic from the topic list or from its detail page. Favorites are pinned at the top of the list (keeping the chosen sort order among themselves), and a new **Favorites only** filter shows just them.
  Favorites are saved per cluster in the local storage of the browser or desktop app, so the browser and desktop modes keep separate lists.

### Changed

- **Faster topic list on large clusters.** The list now renders 200 rows at a time, with a **Show more** button at the bottom. Toggling filters on a cluster with 10,000 topics went from about 2 s to about 50 ms, and the time no longer grows with the number of topics. Search, filters, sorting and the topic count still cover every topic.

[0.3.0]: https://github.com/fededomm/kafkalens/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/fededomm/kafkalens/compare/v0.1.1...v0.2.0
