# Flight2

Flight2 is a high-performance web application designed for browsing and querying tabular data from any cloud service or local filesystem. It integrates powerful libraries to provide a seamless data exploration experience.

## Features

*   **Universal Data Access**: Read files from any cloud provider supported by [Rclone](https://rclone.org/) (S3, GCS, Azure, Dropbox, etc.) or the local filesystem.
*   **Automatic Conversion**: Automatically converts various file formats (CSV, Excel, JSON, HTML tables, ZIP, etc.) into SQLite databases using [mksqlite](https://github.com/darianmavgo/mksqlite).
*   **High-Speed Caching**: Caches gigabytes of converted SQLite databases in memory using [BigCache](https://github.com/allegro/bigcache) for instant query responses.
*   **Secure Secrets Management**: Stores cloud credentials securely in a local, encrypted SQLite database using AES-GCM.
*   **SQL Power**: Query your data using standard SQL via [sqliter](https://github.com/darianmavgo/sqliter).
*   **Banquet URLs**: Use expressive URLs to define data sources and queries.

## Architecture

Flight2 is built with a modular architecture:

*   **`internal/secrets`**: Manages user credentials securely. It encrypts sensitive data (like API keys) before storing them in a local SQLite database (`secrets.db`).
*   **`internal/source`**: leverages `rclone` to fetch file streams from configured cloud backends dynamically.
*   **`internal/dataset`**: Orchestrates data retrieval and conversion. It checks the in-memory cache (`BigCache`) for existing SQLite databases. If not found, it fetches the source file, converts it to SQLite (if necessary), and caches the result.
*   **`internal/server`**: The web server that handles requests. It parses "Banquet" URLs to determine the data source and query, retrieves the appropriate credentials, and streams the query results to the user.

## Usage

### 1. Starting the Server

```bash
# Build and run
go run cmd/server/main.go
```

The server listens on port `8080` by default (configurable via `PORT` environment variable).

### 2. Registering Credentials

Before accessing private cloud data, you must register your credentials. Flight2 generates a secure alias for you to use in URLs.

**Endpoint**: `POST /credentials`

**Payload** (JSON): Rclone configuration map. The `type` field is required.

Example (S3):
```json
{
  "type": "s3",
  "provider": "AWS",
  "access_key_id": "YOUR_ACCESS_KEY",
  "secret_access_key": "YOUR_SECRET_KEY",
  "region": "us-east-1"
}
```

**Response**:
```json
{
  "alias": "abc123xyz"
}
```

### 3. Browsing Data

Access data using the generated alias and the Banquet URL format:

```
http://localhost:8080/<alias>@<source_url>/<table_or_query>
```

**Examples**:

*   **List tables in a remote CSV (converted to DB)**:
    `http://localhost:8080/abc123xyz@mybucket/data.csv`

*   **Query a specific table**:
    `http://localhost:8080/abc123xyz@mybucket/data.xlsx/Sheet1`

*   **Local Filesystem (if enabled/configured)**:
    `http://localhost:8080/@/path/to/local/file.csv` (Note: requires empty alias or specific config)

## Development

### Dependencies

*   [Go 1.25+](https://go.dev/)
*   `gcc` (for SQLite CGO)

### Running Tests

```bash
go test ./...
```

### Directory Structure

*   `cmd/server`: Main entry point.
*   `internal/dataset`: Data conversion and caching logic.
*   `internal/secrets`: Encryption and credential storage.
*   `internal/server`: HTTP handlers and routing.
*   `internal/source`: Rclone integration.
*   `templates`: HTML templates for the UI.

## License

[MIT](LICENSE)
