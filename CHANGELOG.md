# Changelog

All notable changes to KafkaLens are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.6.0] - 2026-11-02

### Added

- **Pick topics for a folder.** **New folder** has a dropdown with a checkbox per topic, a filter and **Select all**. The prefix is now optional and narrows the list: when topics are chosen, only those are saved; with none chosen, the prefix rule is saved as before.
- **Kafka errors are shown.** Browsing a topic you can't read (e.g. missing ACLs) reports the error instead of an empty result, and live tail stops with the message instead of staying silently "live". The Consumers tab of a topic shows when its groups could not be read.
- **Delete all messages of a topic.** **Delete messages** on the topic page empties every partition up to now, after typing the topic name; the topic, its partitions and configuration stay. Disabled on read-only clusters, internal topics and compacted topics (Kafka refuses it when `cleanup.policy` has no `delete`).
- **Binary headers.** Header values that are not UTF-8 travel as base64 (marked *base64*) instead of being corrupted, in the inspector, exports and **Re-send**.

### Changed

- **Confirmations in the app's look.** Deleting a folder, a consumer group or a connection asks in an app dialog instead of the browser's; deleting a topic uses the same dialog with the name to type. Navigating away cancels the question.
- **Re-send copies binary records faithfully**: the original bytes are produced, not their base64 text.
- **Lighter topic page and palette.** The topic page computes consumer lag only for that topic instead of every group of the cluster, and **Ctrl+K** loads only topic and group names.
- **Exports in the desktop app** confirm the saved path and report write errors.
- **Removing a connection** also forgets its favorites and folders.
- The Brokers card in Overview scrolls to the brokers table; sortable columns and *Import JSON* work from the keyboard.

### Fixed

- **Consumer groups with `:` `/` `@` `=` `+` in the name**: details, delete and offset reset now act on the right group (reset used to create a new group with the encoded name).
- **Offset reset**: *Time* without a date no longer resets to the earliest offset, negative offsets are refused, an offset below the log start is moved to it (consumers used to jump to the end), and changing topic resets the partition.
- **Transactional topics**: reading up to the end no longer waits 15 s and reports *partial*.
- **Produce dialog** shows the values it is about to send when reopened or used for Re-send.
- Pages no longer show data of the previous cluster, topic or group when a slow answer arrives after switching; the same for the messages filter while typing.
- Live tail: no runaway memory while paused or in a background tab, and quick Live/Stop toggles no longer leave a stream open.
- Double clicks no longer save a connection twice or send a message twice; Add partitions is disabled while running.
- Clearing the client certificate of a saved connection works; saving a config value without changes no longer sends an empty value; the broker config dialog no longer spins forever on error; *Test connection* results no longer appear in the next connection's dialog; the topic list keeps its length after starring a topic; the Consumers tab and the group list show group states and errors like the rest of the app.
- `clusters.json` is synced to disk before replacing the old file, and a failed save no longer changes the connections in memory.

### Security

- **Browser mode** only answers requests whose `Host` is `localhost` or a loopback IP and blocks cross-origin changes, so other web pages can't reach the API through DNS rebinding or cross-site POSTs. LAN access is not supported.

## [0.5.0] - 2026-11-02

### Added

- **Topic folders.** Group topics into named folders from the folder button on each row of the topic list; a topic can be in several folders. When folders exist, a selector next to the filters shows only the topics of one folder, and the trash button next to it deletes the folder (the topics are not touched). **New folder** creates a folder from a prefix (e.g. `orders.`): it holds every topic whose name starts with it, ignoring case like the search box, including topics created later, and those topics can't be taken out one by one. A folder is removed when its last topic is taken out. Like favorites, folders are saved per cluster in the local storage of the browser or desktop app.

## [0.4.0] - 2026-10-16

### Added

- **Themed date picker.** The date and time fields (the *Time* start position in Messages and the reset-to-time option for consumer groups) open a calendar that matches the app's light and dark themes, replacing the browser's picker. Dates can still be typed, picking a day keeps the time already entered, and **Now** / **Clear** are one click away.
- **Startup settings in `config.yaml`.** Listen address, opening the browser, the clusters file and the log level, read with viper. The browser mode reads `./config.yaml` or `-settings <file>`; every key can also be set with a `KAFKALENS_<SECTION>_<KEY>` environment variable (e.g. `KAFKALENS_SERVER_ADDR`); the existing `-addr`, `-config` and `-no-open` flags still work and take precedence. The desktop app embeds the file at build time and also reads the environment. Unknown keys and invalid values are reported at startup.
- **Version in the sidebar.** The app version is shown at the bottom left, under **Clusters**, and in the startup log line. It comes from `productVersion` in `wails.json`, so a release only bumps it there.
- **Logging.** Readable, aligned log lines (via zerolog), e.g. `10:58:50.375 WRN DELETE /api/c/ro/topics/x error="cluster is read-only" ms=0 status=403`. The browser mode logs to the console (colored on a terminal) and the desktop app also to `kafkalens.log` next to `clusters.json`; at startup a log over 10 MiB is moved to `kafkalens.log.1`. Failed calls and changes are logged with status, duration and error; `log.level: debug` (or `KAFKALENS_LOG_LEVEL=debug`) logs reads too.

### Changed

- **Simpler message search bar.** By default it shows only the text filter, **Fetch** and **Live tail**, and reads the newest 1,000 messages (previously 100). Start position (newest, oldest, offset, time), partition and message count are under **More options**. They apply only while the panel is open: closing it goes back to the defaults, and reopening it restores the previous choices.
- **Export buttons moved.** **Export messages** is now in the topic header, and **Export config** is in the toolbar of the *Configuration* tab.
- The message list no longer shows the keyboard hint or a focus outline. The ↑/↓ (and j/k) keys still move between messages.
- **Backend reorganized.** API routes now run on [chi](https://github.com/go-chi/chi), and the Go code is split into `internal/api`, `internal/kafka`, `internal/store` and `internal/config`. The API paths and responses are unchanged, except that unknown `/api` routes and wrong methods now get a JSON error (404 / 405) instead of a plain-text one.
- **Go 1.27.3.** The project now builds with Go 1.27.3 (previously 1.26.9), as set in `go.mod`; CI and the desktop build pick it up from there.

### Fixed

- The offset and time fields in the message search bar no longer shrink to nothing at some window widths (a regression in 0.3.0).
- Date and time fields use the theme colors instead of the system blue for the selected part.
- The topic list and the overview no longer fail with `TOPIC_AUTHORIZATION_FAILED` after opening a topic or consumer group whose lag involves topics you are not authorized on. They now read fresh metadata instead of the client cache, and start/end offsets are requested only for the listed topics.
- In the consumer group list, the row borders in the *Topics* column line up with the rest of the row again, and group names no longer wrap onto two lines.

### Security

- Updated dependencies flagged by `trivy fs`: `github.com/labstack/echo/v4` 4.16.1 (used by Wails), `golang.org/x/crypto` 0.57.0, `golang.org/x/net` 0.60.0, `golang.org/x/text` 0.42.0, and in the UI `zod` 4.6.6 (via `@angular/cli` / `@angular/build` 22.2.4).

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

[0.6.0]: https://github.com/fededomm/kafkalens/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/fededomm/kafkalens/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/fededomm/kafkalens/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/fededomm/kafkalens/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/fededomm/kafkalens/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/fededomm/kafkalens/compare/v0.1.1...v0.2.0
