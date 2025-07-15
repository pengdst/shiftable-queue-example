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
	StatusFailed
	StatusCompleted
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
	w.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(map[string]any{
		"data": data,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal JSON")
		w.WriteHeader(http.StatusInternalServerError)
		if _, writeErr := w.Write([]byte(`{"error":"internal server error"}`)); writeErr != nil {
			log.Error().Err(writeErr).Msg("failed to write error response")
		}
		return
	}
	w.WriteHeader(status)
	if _, writeErr := w.Write(b); writeErr != nil {
		log.Error().Err(writeErr).Msg("failed to write error response")
	}
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
	w.Header().Set("Content-Type", "application/json")
	// Marshal the actual error object, not just the string.
	// This allows for richer error structures in the JSON response.
	b, marshalErr := json.Marshal(map[string]any{
		"error": err,
	})
	if marshalErr != nil {
		log.Error().Err(marshalErr).Msg("failed to marshal JSON for error response")
		w.WriteHeader(http.StatusInternalServerError)
		if _, writeErr := w.Write([]byte(`{"error":"internal server error"}`)); writeErr != nil {
			log.Error().Err(writeErr).Msg("failed to write error response")
		}
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	if _, writeErr := w.Write(b); writeErr != nil {
		log.Error().Err(writeErr).Msg("failed to write error response")
	}
}
