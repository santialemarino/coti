# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /api
COPY apps/api/go.mod apps/api/go.sum* ./
RUN go mod download
COPY apps/api/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /api/bin/api ./cmd/api

FROM alpine:3.21 AS runner
WORKDIR /api
RUN apk add --no-cache ca-certificates
COPY --from=builder /api/bin/api /api/bin/api
EXPOSE 8000
CMD ["/api/bin/api"]
