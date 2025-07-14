package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type ServerOption func(*Server)

func WithConfig(cfg *Config) ServerOption {
	return func(s *Server) { s.cfg = cfg }
}

func WithDB(db *gorm.DB) ServerOption {
	return func(s *Server) { s.db = db }
}

func WithProcessor(processor QueueTrigger) ServerOption {
	return func(s *Server) { s.processor = processor }
}

func NewServer(opts ...ServerOption) (*Server, error) {
	s := &Server{}
	var err error // Declare err here
	for _, opt := range opts {
		opt(s)
	}
	// Fallback ke default kalau belum diinject
	if s.cfg == nil {
		s.cfg = Load()
	}
	if s.db == nil {
		s.db, err = NewGORM(s.cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create GORM instance: %w", err)
		}
	}
	if err := Migrate(s.db); err != nil {
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	if s.processor == nil {
		s.processor, err = NewQueueProcessor(s.cfg, s.db, &FakeAPI{})
		if err != nil {
			return nil, fmt.Errorf("failed to create queue processor: %w", err)
		}
	}

	repo := NewRepository(s.db)
	handler := NewHttpHandler(repo, s.processor)
	r := http.NewServeMux()
	r.HandleFunc("GET /", rootHandler)
	r.HandleFunc("POST /api/v1/queues", handler.CreateQueue)
	r.HandleFunc("GET /api/v1/queues", handler.GetQueueList)
	r.HandleFunc("DELETE /api/v1/queues/{id}", handler.DeleteQueueByID)
	r.HandleFunc("POST /api/v1/queues/process", handler.ProcessQueue)
	r.HandleFunc("POST /api/v1/queues/leave", handler.LeaveQueue)

	s.router = r
	return s, nil
}

type Server struct {
	router    *http.ServeMux
	cfg       *Config
	db        *gorm.DB
	processor QueueTrigger
}

// Run method of the Server struct runs the HTTP server on the specified port. It initializes
// a new HTTP server instance with the specified port and the server's router.
func (s *Server) Run(port int) error {
	addr := fmt.Sprintf(":%d", port)

	h := chainMiddleware(
		s.router,
		recoverHandler,
		loggerHandler(func(w http.ResponseWriter, r *http.Request) bool { return r.URL.Path == "/health" }),
		realIPHandler,
		requestIDHandler,
		corsHandler,
	)

	httpSrv := http.Server{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-quit
		log.Info().Msg("server is shuting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		httpSrv.SetKeepAlivesEnabled(false)
		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Error().Err(fmt.Errorf("could not gracefully shutdown the server: %w", err)).Msg("server shutdown error")
		}
		close(done)
	}()

	log.Info().Msgf("server serving on port %d", port)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("could not listen on %s: %w", addr, err)
	}

	<-done
	log.Info().Msg("server stopped")
	return nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}
