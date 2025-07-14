package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const QueueProcessingName = "queue_processing"

type RestAPI interface {
	SimulateProcessing(*Queue) error
}

type Publisher interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp091.Publishing) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error)
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp091.Table) (amqp091.Queue, error)
	Close() error
}

type Closer interface {
	Close() error
	Channel() (*amqp091.Channel, error)
}

type QueueProcessor struct {
	repo    *Repository
	conn    Closer
	channel Publisher
	api     RestAPI
}

// processorOptions is an unexported struct to hold optional dependencies for testing.
type processorOptions struct {
	conn    Closer
	channel Publisher
}

func WithConnection(conn Closer) func(*processorOptions) {
	return func(o *processorOptions) {
		o.conn = conn
	}
}

func WithChannel(channel Publisher) func(*processorOptions) {
	return func(o *processorOptions) {
		o.channel = channel
	}
}

func NewQueueProcessor(cfg *Config, db *gorm.DB, api RestAPI, opts ...func(*processorOptions)) (*QueueProcessor, error) {
	options := &processorOptions{}
	for _, opt := range opts {
		opt(options)
	}

	if options.conn == nil {
		// Create real connection and channel (for production)
		if cfg == nil {
			return nil, errors.New("config is required for production setup")
		}
		conn, err := amqp091.Dial(cfg.RabbitMQURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
		}
		options.conn = conn
	}

	if options.channel == nil {
		conn := options.conn

		chann, err := conn.Channel()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to open RabbitMQ channel: %w", err)
		}

		_, err = chann.QueueDeclare(
			QueueProcessingName, // name
			true,                // durable
			false,               // delete when unused
			false,               // exclusive
			false,               // no-wait
			nil,                 // arguments
		)
		if err != nil {
			chann.Close()
			conn.Close()
			return nil, fmt.Errorf("failed to declare RabbitMQ queue: %w", err)
		}
		options.channel = chann
	}

	return &QueueProcessor{
		repo:    NewRepository(db),
		conn:    options.conn,
		channel: options.channel,
		api:     api,
	}, nil
}

func (p *QueueProcessor) Start() error {
	log.Info().Msg("Queue processor started")

	// Start consuming messages from RabbitMQ
	msgs, err := p.channel.Consume(
		QueueProcessingName, // queue
		"",                  // consumer
		false,               // auto-ack
		false,               // exclusive
		false,               // no-local
		false,               // no-wait
		nil,                 // args
	)
	if err != nil {
		return fmt.Errorf("failed to register RabbitMQ consumer: %w", err)
	}

	log.Info().Msg("RabbitMQ consumer started, waiting for messages...")

	for d := range msgs {
		ctx := context.Background()

		if err := p.ProcessEligibleQueue(ctx); err != nil {
			log.Error().Msgf("failed to process eligible queue: %s", err.Error())
			d.Nack(false, true) // requeue
		} else {
			d.Ack(false)
		}
	}
	return nil
}

func (p *QueueProcessor) Stop() {
	if p.channel != nil {
		p.channel.Close()
	}
	if p.conn != nil {
		p.conn.Close()
	}
	log.Info().Msg("Queue processor stopped")
}

func (p *QueueProcessor) TriggerProcessing(ctx context.Context) error {
	message := map[string]string{"trigger": TriggerStartProcessing}
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return p.channel.PublishWithContext(
		ctx,
		"",                  // exchange
		QueueProcessingName, // routing key
		false,               // mandatory
		false,               // immediate
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}

func (p *QueueProcessor) ProcessEligibleQueue(ctx context.Context) error {
	// Get next eligible queue (1 queue only, ordered by shifting logic)
	queues, err := p.repo.GetEligibleQueues(ctx)
	if err != nil {
		return err
	}

	if len(queues) == 0 {
		log.Debug().Msg("no eligible queues to process")
		return nil
	}

	queue := &queues[0]

	log.Info().Msgf("processing queue %s (ID: %d)", queue.Name, queue.ID)

	// Simulate actual processing logic
	if err := p.api.SimulateProcessing(queue); err != nil {
		log.Warn().Msgf("queue %s (ID: %d) failed, retry count: %d", queue.Name, queue.ID, queue.RetryCount)
		queue.Status = StatusFailed
		queue.LastRetryAt = sql.NullTime{Time: time.Now(), Valid: true}
		queue.RetryCount++

		if err := p.repo.Save(ctx, queue); err != nil {
			log.Error().Msgf("failed to update queue %s: %s", queue.Name, err.Error())
			return err
		}
		return err
	}
	queue.Status = StatusCompleted

	// Update queue status in database
	if err := p.repo.Save(ctx, queue); err != nil {
		log.Error().Msgf("failed to update queue %s: %s", queue.Name, err.Error())
		return err
	}

	// Check if there are more eligible queues to process
	exist, err := p.repo.HasRemainingQueues(ctx)
	if err != nil {
		log.Error().Msgf("failed to check remaining queues: %s", err.Error())
		return err
	}

	// If there are more queues, trigger RabbitMQ again for chain processing
	if !exist {
		log.Info().Msg("no more eligible queues, processing complete")
		return nil
	}

	log.Info().Msg("found more eligible queues, triggering next processing")
	if err := p.triggerNextProcessing(ctx); err != nil {
		log.Error().Msgf("failed to trigger next processing: %s", err.Error())
	}

	return nil
}

func (p *QueueProcessor) triggerNextProcessing(ctx context.Context) error {
	message := map[string]string{"trigger": TriggerProcessNext}
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return p.channel.PublishWithContext(
		ctx,
		"",                  // exchange
		QueueProcessingName, // routing key
		false,               // mandatory
		false,               // immediate
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}

type FakeAPI struct {
	randFloat func() float32
}

// SimulateProcessing simulates random success/failure for demo purposes
func (p *FakeAPI) SimulateProcessing(*Queue) error {
	// Use default if not set
	rander := p.randFloat
	if rander == nil {
		rander = rand.Float32
	}
	if rander() < 0.7 {
		return nil
	}
	return errors.New("processing failed")
}
