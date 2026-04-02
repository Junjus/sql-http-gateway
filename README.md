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

Set the following environment variables and then run the application:

```bash
export DB_TYPE=pgx
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=mydb
export DB_USER=user
export DB_PASSWORD=password

go run .
```

Or with flags (flags override environment variables):

```bash
DB_HOST=localhost DB_PORT=5432 DB_NAME=mydb DB_USER=user DB_PASSWORD=password \
  go run . \
  -addr :8080 \
  -sql-dir ./queries
```

## Docker

Build and run the test stack with Docker Compose:

```bash
docker compose -f compose.yaml up --build
```

This starts:

- `db`: a PostgreSQL test database
- `app`: the Go service on `http://localhost:8080`

The compose file initializes the database from `db/init/*.sql`, seeds sample data, mounts `./queries` into the app container, and passes database connection details via environment variables. You can override these in the compose file or via a `.env` file for local testing.

If you want to re-run the seed scripts from scratch, remove the database volume first:

```bash
docker compose -f compose.yaml down -v
```

Flags:

- `-addr`: HTTP bind address (default `:8080`).
- `-sql-dir`: directory with `.sql` files (default `./queries`).
- `-query-timeout`: timeout per query (default `10s`).
- `-ping-timeout`: timeout for startup DB ping (default `5s`).

Environment Variables:

- `DB_TYPE`: database/sql driver name (default `pgx`).
- `DB_HOST`: required, database hostname.
- `DB_PORT`: database port (default `5432`).
- `DB_NAME`: required, database name.
- `DB_USER`: required, database username.
- `DB_PASSWORD`: required, database password.

## Endpoints

- `GET /healthz` -> `{"status":"ok"}`
- `GET /queries` -> list of generated query endpoints
- `GET /<sql-file-name>` -> query result as JSON array

## Notes

- Endpoint names are derived from SQL filenames (without `.sql`) and sanitized.
- Empty SQL files are rejected at startup.
- If two files map to the same endpoint after sanitization, startup fails with an error.
