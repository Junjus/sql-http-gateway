# sqlToJson

Small Go service that reads `.sql` files from a directory and creates one HTTP endpoint per file. Each endpoint executes its SQL query and returns the rows as JSON.

## Features

- Uses Go standard library for HTTP server, JSON handling, and SQL execution.
- Auto-registers one endpoint per `.sql` file.
- Health check endpoint: `/healthz`.
- Query listing endpoint: `/queries`.

## Project Structure

- `main.go`: Server and query loading logic.
- `queries/*.sql`: SQL files mapped to endpoints.

Example:

- `queries/example.sql` -> endpoint `/example`

## Requirements

- Go 1.23+
- A database driver registered with `database/sql` at build time.

Important: Go's standard library includes `database/sql` but does not include concrete database drivers. You must add one (for example PostgreSQL, MySQL, or SQLite) depending on your database.

## Run

```bash
go run . \
  -driver postgres \
  -dsn "postgres://user:password@localhost:5432/mydb?sslmode=disable" \
  -sql-dir ./queries \
  -addr :8080
```

## Docker

Build and run the test stack with Docker Compose:

```bash
docker compose -f compose.yaml up --build
```

This starts:

- `db`: a PostgreSQL test database
- `app`: the Go service on `http://localhost:8080`

The compose file initializes the database from `db/init/*.sql`, seeds sample data, mounts `./queries` into the app container, and points the app at the test database with the `pgx` driver.

If you want to re-run the seed scripts from scratch, remove the database volume first:

```bash
docker compose -f compose.yaml down -v
```

Flags:

- `-driver`: required, database/sql driver name.
- `-dsn`: required, database connection string.
- `-sql-dir`: directory with `.sql` files (default `./queries`).
- `-addr`: HTTP bind address (default `:8080`).
- `-query-timeout`: timeout per query (default `10s`).
- `-ping-timeout`: timeout for startup DB ping (default `5s`).

## Endpoints

- `GET /healthz` -> `{"status":"ok"}`
- `GET /queries` -> list of generated query endpoints
- `GET /<sql-file-name>` -> query result as JSON array

## Notes

- Endpoint names are derived from SQL filenames (without `.sql`) and sanitized.
- Empty SQL files are rejected at startup.
- If two files map to the same endpoint after sanitization, startup fails with an error.
