package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type QueueStatus int

const (
	StatusPending QueueStatus = iota
	StatusProcessing
	StatusCompleted
	StatusFailed
)

func (s QueueStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusProcessing:
		return "processing"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

type Queue struct {
	gorm.Model
	Name        string
	Status      QueueStatus  `gorm:"default:0"`
	LastRetryAt sql.NullTime `gorm:"default:null"`
	RetryCount  int          `gorm:"default:0"`
}

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{
		repo: repo,
	}
}

type httpHandler struct {
	repo *Repository
}

func NewHttpHandler(repo *Repository) *httpHandler {
	return &httpHandler{
		repo: repo,
	}
}

func (h *httpHandler) CreateQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Ctx(ctx).Error().Msgf("failed to decode request: %s", err.Error())
		WriteJSONError(w, err)
		return
	}

	if err := h.repo.Save(ctx, &Queue{
		Name: req.Name,
	}); err != nil {
		log.Ctx(ctx).Error().Msgf("failed to get queues: %s", err.Error())
		WriteJSONError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *httpHandler) GetQueueList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	queues, err := h.repo.Fetch(ctx)
	if err != nil {
		log.Ctx(ctx).Error().Msgf("failed to get queues: %s", err.Error())
		WriteJSONError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, queues)
}

func (h *httpHandler) DeleteQueueByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, _ := strconv.Atoi(r.PathValue("id"))

	err := h.repo.DeleteByID(ctx, id)
	if err != nil {
		log.Ctx(ctx).Error().Msgf("failed to delete queue: %s", err.Error())
		WriteJSONError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.WriteHeader(status)
	byte, _ := json.Marshal(map[string]any{
		"data": data,
	})
	w.Write(byte)
}

func (h *httpHandler) ProcessQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Ctx(ctx).Error().Msgf("failed to decode request: %s", err.Error())
		WriteJSONError(w, err)
		return
	}

	if req.Name == "" {
		WriteJSONError(w, fmt.Errorf("queue name is required"))
		return
	}

	// Get queue by name
	queue, err := h.repo.GetQueueByName(ctx, req.Name)
	if err != nil {
		log.Ctx(ctx).Error().Msgf("failed to get queue by name %s: %s", req.Name, err.Error())
		WriteJSONError(w, err)
		return
	}

	// Check if queue is eligible for processing (pending or failed)
	if queue.Status != StatusPending && queue.Status != StatusFailed {
		WriteJSONError(w, fmt.Errorf("queue %s is not eligible for processing, current status: %s", req.Name, queue.Status.String()))
		return
	}

	// Update status to processing
	queue.Status = StatusProcessing
	if err := h.repo.Save(ctx, queue); err != nil {
		log.Ctx(ctx).Error().Msgf("failed to update queue %s status: %s", req.Name, err.Error())
		WriteJSONError(w, err)
		return
	}

	// TODO: Trigger RabbitMQ to process this queue
	// publishToRabbitMQ(queue)
	log.Ctx(ctx).Info().Msgf("queue %s triggered for processing", req.Name)

	WriteJSON(w, http.StatusOK, map[string]any{
		"message": "queue triggered for processing",
		"name":    req.Name,
		"status":  queue.Status.String(),
	})
}

func WriteJSONError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	byte, _ := json.Marshal(map[string]any{
		"error": err.Error(),
	})
	w.Write(byte)
}
