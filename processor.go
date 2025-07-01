package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type RestAPI interface {
	SimulateProcessing(*Queue) error
}

type QueueProcessor struct {
	repo    *Repository
	conn    *amqp091.Connection
	channel *amqp091.Channel
	api     RestAPI
}

func NewQueueProcessor(cfg *Config, db *gorm.DB, api RestAPI) *QueueProcessor {
	// Connect to RabbitMQ
	conn, err := amqp091.Dial(cfg.RabbitMQURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to RabbitMQ")
	}

	channel, err := conn.Channel()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open RabbitMQ channel")
	}

	// Declare queue
	_, err = channel.QueueDeclare(
		"queue_processing", // name
		true,               // durable
		false,              // delete when unused
		false,              // exclusive
		false,              // no-wait
		nil,                // arguments
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to declare RabbitMQ queue")
	}

	return &QueueProcessor{
		repo:    NewRepository(db),
		conn:    conn,
		channel: channel,
		api:     api,
	}
}

func (p *QueueProcessor) Start() {
	log.Info().Msg("Queue processor started")

	// Start consuming messages from RabbitMQ
	msgs, err := p.channel.Consume(
		"queue_processing", // queue
		"",                 // consumer
		false,              // auto-ack
		false,              // exclusive
		false,              // no-local
		false,              // no-wait
		nil,                // args
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to register RabbitMQ consumer")
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
		"",                 // exchange
		"queue_processing", // routing key
		false,              // mandatory
		false,              // immediate
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
	remainingQueues, err := p.repo.GetEligibleQueues(ctx)
	if err != nil {
		log.Error().Msgf("failed to check remaining queues: %s", err.Error())
		return err
	}

	// If there are more queues, trigger RabbitMQ again for chain processing
	if len(remainingQueues) <= 0 {
		log.Info().Msg("no more eligible queues, processing complete")
		return nil
	}

	log.Info().Msgf("found %d more eligible queues, triggering next processing", len(remainingQueues))
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
		"",                 // exchange
		"queue_processing", // routing key
		false,              // mandatory
		false,              // immediate
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}

type FakeAPI struct{}

// SimulateProcessing simulates random success/failure for demo purposes
func (p *FakeAPI) SimulateProcessing(*Queue) error {
	if rand.Float32() < 0.7 {
		return nil
	}
	return errors.New("processing failed")
}
