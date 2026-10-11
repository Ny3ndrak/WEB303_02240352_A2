# Equipment Bookings — WEB303 Assignment 2

Two collaborating microservices behind an API Gateway, built in Go with gRPC.

- **equipment-service (Service B)** — owns `Equipment` (e.g. projectors, cameras) in its own SQLite database. Independent; calls no one.
- **booking-service (Service A)** — owns `Booking`s in its own SQLite database. Every `CreateBooking` calls equipment-service over gRPC (timeout + retry with backoff + circuit breaker) to confirm the equipment exists before the booking is stored.
- **api-gateway** — the only REST entry point. Forwards to both services over gRPC and maps gRPC status codes to HTTP status codes.

```
Postman --REST--> api-gateway --gRPC--> booking-service --gRPC--> equipment-service
                        |                                              |
                        +-----------------------gRPC--------------------+
                                                                    equipment.db
                                                       bookings.db
```

## Prerequisites

- Go 1.22+
- `protoc` with `protoc-gen-go` and `protoc-gen-go-grpc`
- Docker and Docker Compose

## 1. Generate the gRPC code

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
export PATH="$PATH:$(go env GOPATH)/bin"

mkdir -p proto/equipment proto/bookings
protoc --go_out=proto/equipment --go_opt=paths=source_relative \
       --go-grpc_out=proto/equipment --go-grpc_opt=paths=source_relative \
       proto/equipment.proto
protoc --go_out=proto/bookings --go_opt=paths=source_relative \
       --go-grpc_out=proto/bookings --go-grpc_opt=paths=source_relative \
       proto/bookings.proto
```

This produces `proto/equipment/equipment.pb.go`, `proto/equipment/equipment_grpc.pb.go`, and the same for `bookings`.

## 2. Install Go dependencies

```bash
go mod tidy
```

## 3. Run everything with Docker Compose

```bash
cp .env.example .env
docker compose up --build
```

The gateway is now on `http://localhost:8080`. Import `postman/equipbooking.postman_collection.json` into Postman and run the requests (set `base_url` to `http://localhost:8080` if it isn't already).

## Endpoints (via the gateway)

| Method | Path | Maps to |
|---|---|---|
| POST | `/equipment` | equipment-service `CreateEquipment` |
| GET | `/equipment` | equipment-service `ListEquipment` (optional `?status=`) |
| GET | `/equipment/{id}` | equipment-service `GetEquipment` |
| PUT | `/equipment/{id}` | equipment-service `UpdateEquipment` |
| DELETE | `/equipment/{id}` | equipment-service `DeleteEquipment` |
| POST | `/bookings` | booking-service `CreateBooking` (verifies the equipment via equipment-service) |
| GET | `/bookings` | booking-service `ListBookings` (optional `?status=`) |
| GET | `/bookings/{id}` | booking-service `GetBooking` |
| PUT | `/bookings/{id}` | booking-service `UpdateBooking` |
| DELETE | `/bookings/{id}` | booking-service `DeleteBooking` |

## gRPC → HTTP mapping (api-gateway)

| gRPC code | HTTP status |
|---|---|
| InvalidArgument | 400 |
| NotFound | 404 |
| AlreadyExists | 409 |
| DeadlineExceeded | 504 |
| Unavailable | 503 |
| Internal / other | 500 |

## Resilience (booking-service → equipment-service)

Implemented in `resilience/resilience.go` (unit tested, 7/7 passing) and wired in `booking-service/equipmentclient.go`:

- **Timeout:** 2s per attempt.
- **Retry with backoff:** up to 3 attempts, exponential backoff (100ms, 200ms), only for `Unavailable` / `DeadlineExceeded` — a `NotFound` or `InvalidArgument` fails immediately without wasting retries.
- **Circuit breaker:** opens after 3 consecutive failures, stays open for 5s, then allows one trial (half-open) before closing or re-opening.

To demonstrate this: stop equipment-service (`docker compose stop equipment-service`) and send a `POST /bookings` — the gateway returns `503` with a clear message instead of hanging.

## Validation

Both services validate required fields (`name`, `requester_name`, `equipment_id`) and enum fields (`status`) and reject invalid requests with `InvalidArgument` (→ HTTP 400) without storing anything.
