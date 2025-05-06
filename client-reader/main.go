package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
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
	var option string
	if len(os.Args) > 1 {
		option = strings.Join(os.Args[1:], "")
	}

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

	commandExchange := "replication"
	responseExchange := "response"

	err = ch.ExchangeDeclare(
		commandExchange, // name
		"fanout",        // type
		true,            // durable
		false,           // auto-deleted
		false,           // internal
		false,           // no-wait
		nil,             // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare command exchange: %v", err)
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

	requestID := uuid.New().String()

	responseQueue, err := ch.QueueDeclare(
		requestID, // name
		false,     // durable
		true,      // delete when unused
		true,      // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare response queue: %v", err)
	}

	err = ch.QueueBind(
		responseQueue.Name, // queue name
		requestID,          // routing key
		responseExchange,   // exchange
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to bind response queue: %v", err)
	}

	var query string
	if option == "--all" || option == "-a" {
		query = "SELECT * FROM test_data"
		fmt.Println("Read all data")
	} else {
		query = "SELECT * FROM test_data ORDER BY id DESC LIMIT 1"
		fmt.Println("Read last data")
	}

	readLastMessage := ClientRequest{
		ID:     requestID,
		Query:  query,
		IsRead: true,
	}

	msgBody, err := json.Marshal(readLastMessage)
	if err != nil {
		log.Fatalf("Failed to marshal message: %v", err)
	}

	err = ch.Publish(
		commandExchange, // exchange
		"",              // routing key
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        msgBody,
		})
	if err != nil {
		log.Fatalf("Failed to publish read request: %v", err)
	}

	fmt.Println("Now waiting for a response")

	msgs, err := ch.Consume(
		responseQueue.Name, // queue
		"",                 // consumer
		true,               // auto-ack
		false,              // exclusive
		false,              // no-local
		false,              // no-wait
		nil,                // args
	)
	if err != nil {
		log.Fatalf("Failed to register a consumer: %v", err)
	}

	for msg := range msgs {
		var response ResponseMessage
		if err := json.Unmarshal(msg.Body, &response); err != nil {
			log.Printf("Error parsing response: %v", err)
			log.Printf("Waiting for another response")
			continue
		}
		fmt.Println("response received")
		for _, data := range response.Data {
			fmt.Println(data)
		}
		break
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
