# Changelog

All notable changes to KafkaLens are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Themed date picker.** The date and time fields (the *Time* start position in Messages and the reset-to-time option for consumer groups) open a calendar that matches the app's light and dark themes, replacing the browser's picker. Dates can still be typed, picking a day keeps the time already entered, and **Now** / **Clear** are one click away.

### Changed

- **Simpler message search bar.** By default it shows only the text filter, **Fetch** and **Live tail**, and reads the newest 1,000 messages (previously 100). Start position (newest, oldest, offset, time), partition and message count are under **More options**. They apply only while the panel is open: closing it goes back to the defaults, and reopening it restores the previous choices.
- **Export buttons moved.** **Export messages** is now in the topic header, and **Export config** is in the toolbar of the *Configuration* tab.
- The message list no longer shows the keyboard hint or a focus outline. The ↑/↓ (and j/k) keys still move between messages.

### Fixed

- The offset and time fields in the message search bar no longer shrink to nothing at some window widths (a regression in 0.3.0).
- Date and time fields use the theme colors instead of the system blue for the selected part.

## [0.3.1] - 2026-10-16

### Added

- **Export and import topic configuration.** **Export config** downloads `<topic>-export.json` with the partition count, replication factor and the configs set on the topic itself (values inherited from the broker are left out). In *Create topic*, **Import JSON** fills the dialog from that file, so a topic can be recreated on another cluster. The name is left for you to type, and the cleanup policy and retention selectors still override imported values.

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

[Unreleased]: https://github.com/fededomm/kafkalens/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/fededomm/kafkalens/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/fededomm/kafkalens/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/fededomm/kafkalens/compare/v0.1.1...v0.2.0
