# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /gotunnel-server ./cmd/server
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /gotunnel-client ./cmd/client

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /gotunnel-server /usr/local/bin/
COPY --from=builder /gotunnel-client /usr/local/bin/

EXPOSE 7000

ENTRYPOINT ["gotunnel-server"]
CMD ["--port", "7000"]
