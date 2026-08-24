# gRPC-GO-mini-project

A simple gRPC mini project written in GO.

## Architecture

The code is organised in layers, with dependencies pointing inward: outer
layers know about inner ones, never the reverse.

```
cmd/server           composition root — the only place that names concrete types
  |
  v
internal/transport/  inbound adapter: gRPC <-> domain translation
  |
  v
internal/service/    business logic (AuthService, GameService, ...)
  |
  v
internal/domain/     entities + port interfaces + pure rules. Imports nothing.
  ^
  |
internal/adapter/    outbound adapters implementing the domain's ports:
                       postgres/  durable users
                       redis/     sessions + leaderboard index
                       memory/    in-process chat and board fan-out
                       security/  bcrypt hashing, token generation
```

`internal/domain` declares interfaces (`UserRepository`, `SessionRepository`,
`LeaderboardRepository`, `ChatBroadcaster`, ...) but no implementations. The
services depend only on those interfaces, so they can be unit tested against
in-memory fakes with no Postgres or Redis running:

```bash
go test ./...
```

Choosing which implementation satisfies each interface happens once, in
[cmd/server/main.go](cmd/server/main.go). Changing where something is stored
is a change to that wiring, not to any business logic.

## Log ingestion pipeline

Logs are **not** written one row per RPC. The path is:

```
client ──stream──> Ingest handler ──> queue ──> dispatcher ──> workers ──> Postgres
                   (client streaming)          (500 or 2s)     (bulk + retry)
```

- **Client streaming** (`LogIngest.Ingest`): one open connection carries many
  entries, instead of a request/response round trip per line.
- **Batching**: a batch is cut when it reaches **500 entries** or when
  **2 seconds** pass, whichever happens first, then written with a single
  multi-row `INSERT`.
- **Outage tolerance**: a failed write is retried with exponential backoff and
  never abandoned. While workers retry, batches queue up, and once the queue
  fills `Submit` blocks — pushing backpressure through the gRPC stream to the
  client rather than dropping entries.

### Demonstrating a database outage

Terminal 1 — start everything:

```bash
docker compose up --build
```

Terminal 2 — generate steady log traffic:

```bash
go run ./cmd/logload -rate 200 -duration 40s
```

Terminal 3 — take the database away for ten seconds, then bring it back:

```bash
docker compose pause db && sleep 10 && docker compose unpause db
```

`pause` freezes the container (connections hang) rather than closing it,
which is the more hostile version of the scenario. The server log will show
workers retrying during the outage and then writing the backlog once Postgres
returns. Confirm nothing was lost:

```bash
docker compose exec db psql -U arash -d database -c "SELECT count(*) FROM logs;"
```

The same scenario runs as an automated test, with no Docker required:

```bash
go test ./internal/service/ -run TestSurvivesTenSecondDatabaseOutage -v
```

Use `go test -short ./...` to skip that ten-second test during normal runs.

## Setting up Database (For MacOS)

1. Install PostgreSQL using brew : ```brew install postgresql@16```
2. Run PostgreSQL service using  : ```brew services start postgresql@16```
3. Create database  : ```createdb database```
4. Load the table from SQL file in project : ```psql database -f dbcode.sql```

## Setting up Redis (For MacOS)

The server caches sessions and the leaderboard in Redis.

1. Install Redis using brew : ```brew install redis```
2. Run Redis service using  : ```brew services start redis```

The server connects to ```localhost:6379``` by default; override with the ```REDIS_ADDR``` environment variable if needed.

## Running server and client

1. Install Protoc:
```
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```
2. Download dependencies: ```go mod tidy```
3. Run Server in one terminal: ```go run ./cmd/server```
4. Run Client in another terminal and interact with the server: ```go run ./cmd/client```

## Running with Docker

First, we build using:
```docker compose up --build```

The server and database starts running.
Then we can use ```docker compose run --rm client``` to run a new client instance (multiple instances are runnable)