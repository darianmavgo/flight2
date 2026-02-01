# SQLiter API Documentation

`sqliter` is a Go package for rendering SQLite tables into HTML with minimal dependencies. It supports read-only views as well as basic row-level CRUD (Create, Read, Update, Delete) operations.

## Core Components

### 1. `TableWriter`
The `TableWriter` struct is the main engine for rendering tables. It manages the templates and configuration for table generation.

#### Construction
```go
func NewTableWriter(t *template.Template, cfg *Config) *TableWriter
```
- **t**: A `*template.Template` instance. If `nil`, it falls back to a simple non-templated HTML writer. The package provides embedded default templates via `GetDefaultTemplates()`.
- **cfg**: A `*Config` instance. If `nil`, `DefaultConfig()` is used.

#### Methods

**`StartHTMLTable(w io.Writer, headers []string, title string)`**
- Starts the HTML table rendering.
- Writes the HTML header (including CSS links), table structure, and the table header row.
- **w**: The writer (e.g., `http.ResponseWriter` or `os.Stdout`).
- **headers**: A slice of strings representing the column names.
- **title**: The title of the page/table.

**`WriteHTMLRow(w io.Writer, index int, cells []string) error`**
- Writes a single row of data to the table.
- **index**: The 0-based index of the row (useful for alternating row colors).
- **cells**: A slice of strings representing the cell data for the row. Data order must match `headers`.

**`EndHTMLTable(w io.Writer)`**
- Closes the HTML table and writes the HTML footer (including JS scripts).

**`EnableEditable(editable bool)`**
- Toggles the edit mode for the table.
- If `true`, the `StartHTMLTable` method will set an `X-SQLiter-Editable` header (if writing to `http.ResponseWriter`) and templates may render edit controls.
- Default is `false`.

### 2. Global Functions (Convenience Wrappers)
For simple use cases relying on default configuration and templates.

```go
func StartHTMLTable(w io.Writer, headers []string, title string)
func WriteHTMLRow(w io.Writer, index int, cells []string) error
func EndHTMLTable(w io.Writer)
```

### 3. Configuration
The `Config` struct controls various aspects of the server and renderer.

```go
type Config struct {
    DataDir                 string // Path to SQLite files (default: "sample_data")
    StickyHeader            bool   // Enable sticky table headers (default: true)
    AutoRedirectSingleTable bool   // Redirect to table view if DB has only 1 table (default: true)
    StyleSheet              string // URL path to CSS (default: "/cssjs/default.css")
    Verbose                 bool   // Enable detailed logging (default: false)
    RowCRUD                 bool   // Enable CRUD features (default: false)
    // ... other server-specific fields
}
```

**Loading Config:**
```go
func DefaultConfig() *Config
func LoadConfig(path string) (*Config, error) // Loads from HCL file
```

### 4. Templates
Templates are now **embedded** in the binary. `sqliter.LoadTemplates` has been deprecated/removed in favor of `GetDefaultTemplates`.

```go
func GetDefaultTemplates() *template.Template
```
Returns the standard set of templates (`head.html`, `row.html`, `foot.html`) initialized with necessary helper functions (`json`, `safe`).

## Usage Examples

### Basic Read-Only Table
```go
package main

import (
    "net/http"
    "github.com/darianmavgo/sqliter/sqliter"
)

func handler(w http.ResponseWriter, r *http.Request) {
    headers := []string{"ID", "Name", "Email"}
    
    // using global convenience functions
    sqliter.StartHTMLTable(w, headers, "User List")
    
    sqliter.WriteHTMLRow(w, 0, []string{"1", "Alice", "alice@example.com"})
    sqliter.WriteHTMLRow(w, 1, []string{"2", "Bob", "bob@example.com"})
    
    sqliter.EndHTMLTable(w)
}
```

### Advanced Table with Editing Enabled
```go
func advancedHandler(w http.ResponseWriter, r *http.Request) {
    cfg := sqliter.DefaultConfig()
    cfg.RowCRUD = true // Enable CRUD support in config (optional, logic dependent)
    
    // Get embedded templates
    tpl := sqliter.GetDefaultTemplates()
    
    tw := sqliter.NewTableWriter(tpl, cfg)
    tw.EnableEditable(true) // Enable edit controls in render
    
    headers := []string{"ID", "Product", "Price"}
    tw.StartHTMLTable(w, headers, "Product Inventory")
    
    // ... write rows ...
    
    tw.EndHTMLTable(w)
}
```
