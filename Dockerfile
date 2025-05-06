FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY ./replica ./replica

RUN CGO_ENABLED=0 GOOS=linux go build -o /replica ./replica

FROM postgres:17-alpine

COPY --from=builder /replica /usr/local/bin/replica

COPY ./init-db.sh /docker-entrypoint-initdb.d/

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

EXPOSE 5432

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]