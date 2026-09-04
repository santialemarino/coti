# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /api
COPY apps/api/go.mod apps/api/go.sum* ./
RUN go mod download
COPY apps/api/ .
# Every cmd/ package, so a binary added later ships without anyone having to remember this line.
RUN CGO_ENABLED=0 GOOS=linux go build -o /api/bin/ ./cmd/...
# The migration job runs goose out of this image; its version is the tool pin in go.mod. Postgres
# is the only target there will ever be, so the drivers for the others are left out.
RUN CGO_ENABLED=0 GOOS=linux go build \
  -tags='no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' \
  -o /api/bin/goose github.com/pressly/goose/v3/cmd/goose

FROM alpine:3.21 AS runner
WORKDIR /api
RUN apk add --no-cache ca-certificates
COPY --from=builder /api/bin/ /api/bin/
COPY --from=builder /api/migrations/ /api/migrations/
EXPOSE 8000
CMD ["/api/bin/api"]
