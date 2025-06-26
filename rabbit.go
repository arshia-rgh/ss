package main

import (
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"time"
)

func InitRabbit(rabbitURL string) (*amqp.Channel, *amqp.Connection, error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	return ch, conn, nil
}

func Consume(ch *amqp.Channel, queueName string, consumerTimeout time.Duration) (<-chan amqp.Delivery, error) {
	queue, err := ch.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		amqp.Table{
			amqp.ConsumerTimeoutArg: consumerTimeout.Milliseconds(),
		}, // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	err = ch.Qos(3, 0, false) // Prefetch count: 3, Prefetch size: 0, Global: false
	if err != nil {
		return nil, fmt.Errorf("failed to set the qos: %w", err)
	}
	msgs, err := ch.Consume(
		queue.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to consume: %w", err)
	}
	return msgs, nil
}

func Publish(ch *amqp.Channel, queueName string, message any, consumerTimeout time.Duration) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error marshalling message: %w", err)
	}

	queue, err := ch.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		amqp.Table{
			amqp.ConsumerTimeoutArg: consumerTimeout.Milliseconds(),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	err = ch.Publish(
		"",
		queue.Name,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
		})
	if err != nil {
		return fmt.Errorf("failed to publish message: %w, id: %s", err, message)
	}

	return nil
}
