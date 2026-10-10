# Multi-stage build for api-gateway
FROM golang:1.22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache gcc musl-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /out/api-gateway ./api-gateway

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/api-gateway ./api-gateway
ENTRYPOINT ["./api-gateway"]
