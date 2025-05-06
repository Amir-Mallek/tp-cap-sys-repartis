package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/streadway/amqp"
)

type ClientRequest struct {
	ID     string `json:"id"`
	Query  string `json:"query"`
	IsRead bool   `json:"is_read"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("You need to a provide a query.")
		return
	}

	query := strings.Join(os.Args[1:], " ")

	rabbitMQURI := getEnv("RABBITMQ_URI", "amqp://guest:guest@localhost:5672/")

	conn, err := amqp.Dial(rabbitMQURI)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %v", err)
	}
	defer ch.Close()

	err = ch.ExchangeDeclare(
		"replication", // name
		"fanout",      // type
		true,          // durable
		false,         // auto-deleted
		false,         // internal
		false,         // no-wait
		nil,           // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare an exchange: %v", err)
	}

	replicationMsg := ClientRequest{
		ID:     uuid.New().String(),
		Query:  query,
		IsRead: false,
	}

	msgBody, err := json.Marshal(replicationMsg)
	if err != nil {
		log.Fatalf("Failed to marshal message: %v", err)
	}

	err = ch.Publish(
		"replication", // exchange
		"",            // routing key
		false,         // mandatory
		false,         // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        msgBody,
		})
	if err != nil {
		log.Fatalf("Failed to publish a message: %v", err)
	}

	log.Printf("Successfully published SQL query: %s", query)
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
