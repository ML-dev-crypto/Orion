# Orion V1 — Core Flag Engine

A working feature-flag REST API. In-memory by default so you can run it
right now with zero setup; Postgres backend included for when you're ready.

## Run it now (in-memory, no dependencies)

```
go run ./cmd/server
```

Server starts on `:8080`. Try it:

```
curl -X POST localhost:8080/flags -d '{"key":"new_checkout","enabled":true,"rollout_pct":50}'
curl localhost:8080/flags/new_checkout
curl "localhost:8080/flags/new_checkout/evaluate?user_id=alice"
curl localhost:8080/flags
curl -X DELETE localhost:8080/flags/new_checkout
```

## Switch to Postgres

1. Fetch the driver (needs real internet access, not available in the
   sandbox this was built in):
   ```
   go get github.com/lib/pq
   ```
2. Start Postgres locally:
   ```
   docker compose up -d
   ```
   This also runs `migrations/001_create_flags.sql` automatically on
   first boot.
3. In `cmd/server/main.go`, replace:
   ```go
   backend := store.NewMemoryStore()
   ```
   with:
   ```go
   backend, err := store.NewPostgresStore("postgres://orion:orion@localhost:5432/orion?sslmode=disable")
   if err != nil {
       log.Fatal(err)
   }
   ```

Nothing else changes — `api.NewHandler(backend)` and every handler still
work identically, because they only ever talk to the `store.Storage`
interface, never to a concrete type. That's the payoff of the interface
we designed.

## Project layout

```
cmd/server/main.go       — entry point, wires everything together
internal/flags/model.go  — the Flag struct (pure data, no dependencies)
internal/store/          — Storage interface + MemoryStore + PostgresStore
internal/api/handler.go  — HTTP layer, the only package that knows about
                            both flags and store
```

Why `api` is separate from `flags`: `store` needs to import `flags` (to
know what a `Flag` is), so `flags` can never import `store` back without
creating an import cycle. The HTTP handler needs both, so it lives in
its own package.

## What's next (V2, per the roadmap)

- User targeting rules (country, cohort) beyond percentage rollout
- Scheduling (enable at a future timestamp)
- Audit log on every write
- A thin SDK wrapping these HTTP calls

## Go concepts used here, if you want to go read up on any of them

- **Structs + JSON tags** — `model.go`
- **Interfaces (implicit satisfaction)** — `store.go`, the whole reason
  Postgres and in-memory are swappable
- **Pointer receivers** (`func (s *MemoryStore) Get(...)`) — methods
  that need to mutate the struct take a pointer receiver
- **sync.RWMutex** — `memory.go`, needed because concurrent HTTP
  requests are concurrent goroutines hitting a shared map
- **Multiple return values + explicit error checking** — every method
  returns `(value, error)` instead of throwing
- **defer** — cleanup code (`rows.Close()`, `mu.Unlock()`) that runs
  when the function returns, no matter which path it took
- **context.Context** — threaded through every method for future
  cancellation/timeout support
