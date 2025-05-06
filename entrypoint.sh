#!/bin/sh
set -e

# Start PostgreSQL in the background
docker-entrypoint.sh postgres &

# Wait for PostgreSQL to be ready
until pg_isready -h localhost -p 5432 -U postgres; do
  echo "Waiting for PostgreSQL to start..."
  sleep 1
done

echo "PostgreSQL started successfully"

# Start the replica program
echo "Starting replica program..."
exec replica &

# Keep the container running
wait