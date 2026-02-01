# Flight2 & SQLiter Integration

This document summarizes how `flight2` currently utilizes the `sqliter` package for rendering HTML tables.

## Overview

`flight2` delegates the responsibility of rendering SQLite query results and table lists to `sqliter`. It uses `sqliter` primarily as a **view engine**, while `flight2` handles the routing, data retrieval (via `banquet` and `mksqlite`), and business logic.

## Integration Points

### 1. Initialization

In `internal/server/server.go`, the `Server` struct holds a reference to a `*sqliter.TableWriter`.

```go
// internal/server/server.go

func NewServer(...) *Server {
    // 1. Get Default Templates (Embedded)
    t := sqliter.GetDefaultTemplates()

    // 2. Configure SQLiter
    sqliterCfg := sqliter.DefaultConfig()
    sqliterCfg.Verbose = verbose
    // Flight2 manages its own serving of CSS/JS, but sqliter templates reference them.
    // Default stylesheet path in sqliter config is "/cssjs/default.css".

    // 3. Initialize TableWriter
    srv := &Server{
        // ...
        tableWriter: sqliter.NewTableWriter(t, sqliterCfg),
        // ...
    }
    return srv
}
```

### 2. Table Rendering

`flight2` uses `sqliter` to render two types of views:

#### A. Table Listing (Database Content)
When a user navigates to a database root (e.g., `/my-alias@url/`), `flight2` lists all tables in that database.

*   **Method**: `server.listTables`
*   **Usage**:
    ```go
    s.tableWriter.StartHTMLTable(w, headers, "Flight2 Tables")
    // ... iterate tables ...
    s.tableWriter.WriteHTMLRow(w, i, []string{linkHtml, "Table"})
    s.tableWriter.EndHTMLTable(w)
    ```

#### B. Query Results (Table Content)
When a user views a specific table or query result.

*   **Method**: `server.queryTable`
*   **Usage**:
    ```go
    s.tableWriter.StartHTMLTable(w, columns, bq.Table)
    // ... iterate rows ...
    s.tableWriter.WriteHTMLRow(w, rowCounter, strValues)
    s.tableWriter.EndHTMLTable(w)
    ```

## Customization & Styles

*   **Templates**: `flight2` uses the built-in embedded templates from `sqliter` (`head.html`, `row.html`, `foot.html`).
*   **Styles**: `flight2` serves the CSS expected by `sqliter` at `/cssjs/default.css`. This file contains both `sqliter`'s default styles and `flight2`'s application-specific styles (e.g., for the management dashboard).

## Current Limitations

*   **Read-Only**: `flight2` does not currently invoke `TableWriter.EnableEditable(true)`. All table views rendered by `sqliter` in `flight2` are currently read-only, even if `RowCRUD` config might be present in `sqliter`.
*   **Testing**: Integration tests in `tests/` (e.g., `cf_r2_test.go`) define a local helper `loadTemplates` to parse templates from disk for testing purposes, mimicking `sqliter`'s behavior, while the main application uses the embedded templates.
