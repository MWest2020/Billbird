FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o billbird ./cmd/billbird

FROM alpine:3.22

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 1000 billbird

COPY --from=builder /build/billbird /usr/local/bin/billbird
COPY --from=builder /build/migrations /migrations

USER billbird

EXPOSE 8080

ENTRYPOINT ["billbird"]
