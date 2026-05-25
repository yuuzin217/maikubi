# Build stage
FROM golang:1.20-alpine AS builder
WORKDIR /app
COPY server.go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server server.go

# Run stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
