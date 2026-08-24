FROM golang:1.27-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 \
    go build -o main ./cmd/main.go

FROM alpine:3.24

RUN adduser -D -u 1001 aegis

WORKDIR /app

COPY --from=builder --chown=aegis:aegis /build/main .

USER aegis

ENTRYPOINT ["/app/main"]

