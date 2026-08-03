FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 \
    go build -o main ./cmd/main.go

FROM alpine:3.24

WORKDIR /app

COPY --from=builder /build/main .

ENTRYPOINT ["/app/main"]

