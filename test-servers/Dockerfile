# Build stage
FROM golang:alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY test-servers/server.go ./test-servers/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./test-servers/server.go

# Run stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
