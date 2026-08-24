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