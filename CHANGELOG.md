# Changelog

All notable changes to this project are documented in this file.

## [v1.1.0] - 2026-02-25

### Added
- New built-in `format` validators: `URI`, `FILE`, `DIR`, `HOSTPORT`, `UUID`, `IP`, `HEX`, `ALPHANUMERIC`, and `IDENTIFIER`.
- Extended parsing support for `bool`, `int64`, `uint`, `time.Duration`, and custom `encoding.TextUnmarshaler` types.
- New GitHub Actions workflows for CI and `go test` status.

### Changed
- Improved `Load` input and validation error messages for clearer field/env context.
- Strengthened tag handling to reject malformed or empty `env` tags.
- Refactored tests to true table-driven cases for easier extension and maintenance.
- Expanded README docs and improved examples output readability.

## [v1.0.1] - 2025-03-29

### Changed
- Internal package/module cleanup.

## [v1.0.0] - 2025-03-29

### Added
- Initial public release of `simpleenv`.
