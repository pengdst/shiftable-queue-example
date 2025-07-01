package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	TriggerStartProcessing = "start_processing"
	TriggerProcessNext     = "process_next"
)

type QueueTrigger interface {
	TriggerProcessing(ctx context.Context) error
}

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
	repo      *Repository
	processor QueueTrigger
}

func NewHttpHandler(repo *Repository, processor QueueTrigger) *httpHandler {
	return &httpHandler{
		repo:      repo,
		processor: processor,
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
		log.Ctx(ctx).Error().Msgf("failed to decode request body: %s", err.Error())
		WriteJSONError(w, err)
		return
	}
	queue, err := h.repo.GetQueueByName(ctx, req.Name)
	if err != nil {
		log.Ctx(ctx).Error().Msgf("failed to get queue: %s", err.Error())
		WriteJSONError(w, err)
		return
	}
	queue.Status = StatusProcessing
	err = h.repo.Save(ctx, queue)
	if err != nil {
		log.Ctx(ctx).Error().Msgf("failed to save queue: %s", err.Error())
		WriteJSONError(w, err)
		return
	}

	// Trigger processing via processor
	if err := h.processor.TriggerProcessing(ctx); err != nil {
		log.Ctx(ctx).Error().Msgf("failed to trigger processing: %s", err.Error())
		WriteJSONError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"message": "queue processing triggered",
	})
}

func (h *httpHandler) LeaveQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Ctx(ctx).Error().Msgf("failed to decode request body: %s", err.Error())
		WriteJSONError(w, err)
		return
	}

	err := h.repo.DeleteByName(ctx, req.Name)
	if err != nil {
		log.Ctx(ctx).Error().Msgf("failed to save queue: %s", err.Error())
		WriteJSONError(w, err)
		return
	}

	// Trigger processing via processor
	if err := h.processor.TriggerProcessing(ctx); err != nil {
		log.Ctx(ctx).Error().Msgf("failed to trigger processing: %s", err.Error())
		WriteJSONError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"message": fmt.Sprintf("queue %s deleted", req.Name),
	})
}

func WriteJSONError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	byte, _ := json.Marshal(map[string]any{
		"error": err.Error(),
	})
	w.Write(byte)
}
