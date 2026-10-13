# Changelog

All notable changes to KafkaLens are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [0.2.0] - 2026-10-13

### Added

- **Favorite topics.** Star a topic from the topic list or from its detail page. Favorites are pinned at the top of the list (keeping the chosen sort order among themselves), and a new **Favorites only** filter shows just them.
  Favorites are saved per cluster in the local storage of the browser or desktop app, so the browser and desktop modes keep separate lists.

### Changed

- **Faster topic list on large clusters.** The list now renders 200 rows at a time, with a **Show more** button at the bottom. Toggling filters on a cluster with 10,000 topics went from about 2 s to about 50 ms, and the time no longer grows with the number of topics. Search, filters, sorting and the topic count still cover every topic.

[0.2.0]: https://github.com/fededomm/kafkalens/compare/v0.1.1...v0.2.0
