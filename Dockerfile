# Build stage
FROM golang:1.25.6 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

# CGO enabled for SQLite
RUN CGO_ENABLED=1 go build -o messengerserver .

# Runtime stage
FROM debian:bookworm-slim

# Install runtime dependencies for sqlite3 and ca-certificates
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/messengerserver .

# Copy frontend folder
COPY --from=builder /app/frontend ./frontend/

# Create volume directory for database with proper permissions
RUN mkdir -p /app/data && chmod 755 /app/data

# Expose port
EXPOSE 50505

# Set the entrypoint
ENTRYPOINT ["./messengerserver"]