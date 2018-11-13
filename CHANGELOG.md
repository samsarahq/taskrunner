# Changelog

## [Unreleased]

### Added

- `buildkitereporter` for reporting which tasks failed on buildkite ([#7](https://github.com/samsarahq/taskrunner/pull/7)).
- `cache` for caching task invalidations in between taskrunner runs ([#6](https://github.com/samsarahq/taskrunner/pull/6)).
- Tasks can be registered to a [`Registry`](https://godoc.org/github.com/samsarahq/taskrunner#Registry) (accessible via `taskrunner.Add`/`taskrunner.Group`).

### Changed

- Extracted default stdout logging into `clireporter` ([#7](https://github.com/samsarahq/taskrunner/pull/7))
- `RunOption`s now get [`*Runtime`](https://godoc.org/github.com/samsarahq/taskrunner#Runtime), through which the lifecycle of `taskrunner` can be accessed.
