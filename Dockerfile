# Dockerfile for WhatsApp Server
FROM golang:1.23-alpine AS builder

# Install required packages for CGO
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o main .

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates sqlite

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Create data directory for SQLite
RUN mkdir -p /root/data

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main"]
