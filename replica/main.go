package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/lib/pq"
	"github.com/streadway/amqp"
)

type ClientRequest struct {
	ID     string `json:"id"`
	Query  string `json:"query"`
	Source string `json:"source"`
	IsRead bool   `json:"is_read"`
}

type TestData struct {
	ID        int       `json:"id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

type ResponseMessage struct {
	RequestID string     `json:"request_id"`
	Data      []TestData `json:"data"`
}

func main() {
	log.Println("Starting replica service...")

	rabbitMQURI := getEnv("RABBITMQ_URI", "amqp://guest:guest@rabbitmq:5672/")
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("POSTGRES_USER", "postgres")
	dbPassword := getEnv("POSTGRES_PASSWORD", "postgres")
	dbName := getEnv("POSTGRES_DB", "replicadb")
	replicaID := getEnv("REPLICA_ID", "replica1")

	pgConnStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName,
	)

	var db *sql.DB
	var err error
	for i := 0; i < 30; i++ {
		db, err = sql.Open("postgres", pgConnStr)
		if err == nil {
			err = db.Ping()
			if err == nil {
				break
			}
		}
		log.Printf("Failed to connect to database, retrying in 2 seconds...")
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("Connected to PostgreSQL database")

	var conn *amqp.Connection
	for i := 0; i < 30; i++ {
		conn, err = amqp.Dial(rabbitMQURI)
		if err == nil {
			break
		}
		log.Printf("Failed to connect to RabbitMQ, retrying in 2 seconds...")
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()
	log.Println("Connected to RabbitMQ")

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %v", err)
	}
	defer ch.Close()

	requestExchange := "replication"
	responseExchange := "response"

	err = ch.ExchangeDeclare(
		requestExchange,
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to declare an exchange: %v", err)
	}

	err = ch.ExchangeDeclare(
		responseExchange, // name
		"direct",         // type
		true,             // durable
		false,            // auto-deleted
		false,            // internal
		false,            // no-wait
		nil,              // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare response exchange: %v", err)
	}

	queueName := "replica_" + replicaID
	q, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	err = ch.QueueBind(
		q.Name,          // queue name
		"",              // routing key
		requestExchange, // exchange
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to bind a queue: %v", err)
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		log.Fatalf("Failed to register a consumer: %v", err)
	}

	log.Printf("Replica %s is now listening for messages...", replicaID)

	for msg := range msgs {
		var clientRequest ClientRequest
		if err := json.Unmarshal(msg.Body, &clientRequest); err != nil {
			log.Printf("Error parsing message: %v", err)
			msg.Nack(false, false)
			continue
		}

		if clientRequest.IsRead {
			rows, err := db.Query(clientRequest.Query)
			if err != nil {
				log.Printf("Error executing read query: %v", err)
				msg.Nack(false, true)
				continue
			}

			var data []TestData

			for rows.Next() {
				var row TestData
				if err := rows.Scan(&row.ID, &row.Key, &row.Value, &row.CreatedAt); err != nil {
					log.Printf("Row scan error: %v", err)
					continue
				}
				data = append(data, row)
			}

			readLastMessage := ResponseMessage{
				RequestID: clientRequest.ID,
				Data:      data,
			}

			msgBody, err := json.Marshal(readLastMessage)
			if err != nil {
				log.Fatalf("Failed to marshal message: %v", err)
			}

			ch.Publish(
				responseExchange, // exchange
				clientRequest.ID, // routing key
				false,            // mandatory
				false,            // immediate
				amqp.Publishing{
					ContentType: "application/json",
					Body:        msgBody,
				})
		} else {
			log.Printf("Received SQL writing query: %s", clientRequest.Query)

			_, err := db.Exec(clientRequest.Query)
			if err != nil {
				log.Printf("Error executing query: %v", err)
				var pqErr *pq.Error
				if errors.As(err, &pqErr) {
					log.Printf("PostgreSQL error code: %s", pqErr.Code)
				}
				msg.Nack(false, true)
				continue
			}

			log.Printf("Successfully executed write query")
		}

		msg.Ack(false)
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
