FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git and build dependencies
RUN apk add --no-cache git

# Copy Go modules files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY ./replica ./replica

# Build the replica program
RUN CGO_ENABLED=0 GOOS=linux go build -o /replica ./replica

# Final stage
FROM postgres:17-alpine

# Copy the compiled replica binary from the builder stage
COPY --from=builder /replica /usr/local/bin/replica

# Copy initialization script
COPY ./init-db.sh /docker-entrypoint-initdb.d/

# Set environment variables
ENV POSTGRES_USER=postgres
ENV POSTGRES_PASSWORD=postgres
ENV POSTGRES_DB=replicadb
ENV RABBITMQ_URI=amqp://guest:guest@rabbitmq:5672/
ENV DB_HOST=localhost
ENV DB_PORT=5432
ENV DB_USER=postgres
ENV DB_PASSWORD=postgres
ENV DB_NAME=replicadb
ENV REPLICA_ID=replica1

# Expose PostgreSQL port
EXPOSE 5432

# Use an entrypoint script to start both PostgreSQL and the replica program
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]