package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type Queue struct {
	gorm.Model
	Name        string
	LastRetryAt sql.NullTime `gorm:"default:null"`
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

func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	byte, _ := json.Marshal(map[string]any{
		"data": data,
	})
	w.Write(byte)
}

func WriteJSONError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	byte, _ := json.Marshal(map[string]any{
		"error": err.Error(),
	})
	w.Write(byte)
}
