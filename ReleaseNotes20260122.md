# Release Notes - January 22, 2026

## Summary
This release includes significant improvements to the `sqliter` UI, expanded query handling in `banquet`, build fixes in `mksqlite`, and new integration tests in `flight`.

## Changes by Repository

### sqliter
- **Sticky Table Headers**: Added "Sticky Header" feature to HTML tables.
    - Headers now stick to the top of the viewport during scrolling.
    - Configurable via `StickyHeader` in `Config` (default: `true`).
    - Fixed CSS conflict with Bootstrap Table (`overflow: visible` fix).
    - Applied `border-collapse: separate` for better rendering.
- **Auto-Redirect**: Added automatic redirection for single-table databases.
    - Visiting a database root URL now redirects to the table view if only one table exists.
    - Configurable via `AutoRedirectSingleTable` in `Config` (default: `true`).
    - Fixed HTTP header write errors during redirect.

### banquet
- **OrderBy Logic**: Expanded `parseOrderBy` functionality.
    - Now accepts `ColumnPath` and `Query`.
    - Supports sort prefixes (`+`, `-`, `^`, `!^`) in `ColumnPath`.
    - Updated `ParseBanquet` to integrate this logic.

### mksqlite
- **Build Fixes**: Resolved Go module import path issues.
    - Fixed `package ... is not in std` errors.
    - Corrected import paths in `cmd/mksqlite/main.go`.

### flight / flight2
- **Integration Tests**: Added `flight_test.go`.
    - Imports and verifies functionality of sibling repositories (`mksqlite`, `sqliter`, `banquet`, `TableTypeMaster`).
    - Ensures local `go.mod` replacements are working correctly.
- **Module Configuration**: Fixed `go.mod` module path declarations.

## Verification
- All `sqliter` tests passed.
- `sqliter` auto-redirect verified with browser automation.
- `sqliter` sticky header verified with browser automation and CSS inspection.
