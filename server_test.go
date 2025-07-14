package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_rootHandler(t *testing.T) {
	t.Run("POSITIVE-RootPath_ReturnsStatusOK", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		rootHandler(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, http.StatusText(http.StatusOK), rr.Body.String())
	})

	t.Run("NEGATIVE-NonRootPath_ReturnsStatusNotFound", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/something-else", nil)
		rr := httptest.NewRecorder()

		rootHandler(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Equal(t, http.StatusText(http.StatusNotFound)+"\n", rr.Body.String())
	})
}

func TestServer_Run_GracefulShutdown(t *testing.T) {
	// We need a test DB and processor for NewServer to work without real dependencies
	db, cleanupDB := setupTestDatabase(t)
	defer cleanupDB()

	// We can't use the real processor because it will try to connect to RabbitMQ
	// We use a mock/fake one instead. The NoopPublisher and FakeAPI are defined in queue_test.go
	// but are available here because they are in the same package.
	processor := &QueueProcessor{
		repo:    NewRepository(db),
		channel: &NoopPublisher{},
		api:     &FakeAPI{},
	}

	server, err := NewServer(WithDB(db), WithProcessor(processor))
	assert.NoError(t, err)

	// Channel to listen for the server.Run error
	errCh := make(chan error, 1)

	go func() {
		// Use a non-standard port to avoid conflicts during tests
		errCh <- server.Run(8989)
	}()

	// Give the server a moment to start up
	time.Sleep(200 * time.Millisecond)

	// Get the current process and send a shutdown signal
	p, err := os.FindProcess(os.Getpid())
	assert.NoError(t, err)
	err = p.Signal(syscall.SIGTERM)
	assert.NoError(t, err)

	// Wait for the server to shut down, with a timeout to prevent the test from hanging
	select {
	case err := <-errCh:
		// On graceful shutdown, server.Run should return nil.
		assert.NoError(t, err, "server.Run should return nil on graceful shutdown")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to shut down gracefully")
	}
}

func TestNewServer_WithConfig(t *testing.T) {
	t.Run("POSITIVE-ConfigOptionSetsServerConfig", func(t *testing.T) {
		// Create a dummy config
		dummyConfig := &Config{
			Host: "test-host",
			Port: 1234,
		}

		// Create mock DB and processor
		db, cleanupDB := setupTestDatabase(t)
		defer cleanupDB()
		processor := &QueueProcessor{
			repo:    NewRepository(db),
			channel: &NoopPublisher{},
			api:     &FakeAPI{},
		}

		// Create a new server with the WithConfig option, and also provide mock DB and processor
		server, err := NewServer(WithConfig(dummyConfig), WithDB(db), WithProcessor(processor))
		assert.NoError(t, err)

		// Assert that the server's config is the dummy config
		assert.Equal(t, dummyConfig, server.cfg)
	})
}
