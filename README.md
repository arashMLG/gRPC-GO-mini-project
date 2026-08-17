# gRPC-GO-mini-project

A simple gRPC mini project written in GO.


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