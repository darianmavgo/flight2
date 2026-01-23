# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]
### Added
- **SecureBrowse Migration**: Work in progress to migrate SecureBrowse UI and logic into the main server.

## [2026-01-22]
### Added
- **Rclone Converter**: Added support for `rclone` remote paths in `mksqlite` converter.
- **Auto-Select tb0**: Implemented feature to automatically select `tb0` table if it is the only table in `flight2`/`sqliter`.

### Changed
- **Dynamic Page Titles**: HTML page titles now accurately reflect the full URL path.
- **CSV Converter**: Fixed failing tests involved in CSV conversion.

### Fixed
- **CSV Tests**: Resolved compilation errors and test failures in `advanced_test.go`.

## [2026-01-21]
### Added
- **Auto-Redirect**: Added `AutoRedirectSingleTable` config option to redirect to the table view if a database has only one table (enabled by default).

### Changed
- **Sticky Headers**: Improved table header behavior to stick to the top of the viewport.

## [2026-01-20]
### Added
- **Integration Tests**: Added `flight_test.go` to verify imports and functionality of sibling repositories (`mksqlite`, `sqliter`, `banquet`).

### Fixed
- **Go Module Path**: Fixed `go.mod` module path declarations to allow correct `go get` behavior.
- **Build Output**: Resolved issues where binaries were not being created in the expected location.
- **Integration Tests**: Fixed build errors and package references in `tests/integration_test.go`.

## [2026-01-18]
### Changed
- **OrderBy Logic**: Expanded `banquet`'s `parseOrderBy` to support `ColumnPath` and `Query` parameters, and sort prefixes (`+`, `-`, `^`, `!^`).

### Fixed
- **File Serving**: Troubleshooted and fixed `http.FileServer` conflicts with HTML data conversion in `core/parse.go`.

## [2026-01-16]
### Added
- **Fast Zip Parsing**: Integrated optimized Central Directory parsing for ZIP files in `converters/zip` to handle large files more efficiently.
- **Writer Generation**: Implemented generation of `Writer` from `outputPath` in `converters`.

### Fixed
- **Mksqlite Imports**: Fixed package import paths in `mksqlite` to align with module definitions.
- **Documentation**: Updated outdated `README.md` files across the project.
