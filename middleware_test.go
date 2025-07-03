package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestLoggerHandler(t *testing.T) {
	h := loggerHandler(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest("GET", "/", strings.NewReader(`{"foo":"bar"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecoverHandler(t *testing.T) {
	t.Run("POSITIVE-Panic", func(t *testing.T) {
		h := recoverHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("blub")
		}))
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
	t.Run("POSITIVE-NoPanic", func(t *testing.T) {
		h := recoverHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
	t.Run("NEGATIVE-ErrAbortHandler", func(t *testing.T) {
		defer func() {
			if err := recover(); err != http.ErrAbortHandler {
				t.Errorf("recover panic is not ErrAbortHandler: %v", err)
			}
		}()
		h := recoverHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(http.ErrAbortHandler)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestCorsHandler(t *testing.T) {
	t.Run("POSITIVE-NormalRequest", func(t *testing.T) {
		h := corsHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("POSITIVE-OptionsPreflight", func(t *testing.T) {
		h := corsHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.FailNow() // should not be called
		}))
		req := httptest.NewRequest("OPTIONS", "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	})
}

func TestRequestIDHandler(t *testing.T) {
	t.Run("POSITIVE-WithoutRequestID", func(t *testing.T) {
		h := requestIDHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, w.Header().Get("X-Request-Id"))
	})

	t.Run("POSITIVE-WithRequestID", func(t *testing.T) {
		h := requestIDHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Request-Id", "test-id")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "test-id", w.Header().Get("X-Request-Id"))
	})
}

func TestRealIPHandler(t *testing.T) {
	t.Run("POSITIVE-NoRealIPHeader", func(t *testing.T) {
		h := realIPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POSITIVE-WithXRealIP", func(t *testing.T) {
		h := realIPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "1.2.3.4", r.RemoteAddr)
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Real-IP", "1.2.3.4")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func Test_formatReqBody(t *testing.T) {
	t.Run("POSITIVE-ValidJSON_ReturnJSON", func(t *testing.T) {
		result := formatReqBody(&http.Request{}, []byte(`{"yolo": 1}`))

		assert.Equal(t, `{"yolo":1}`, result)
	})
	t.Run("NEGATIVE-StringValue_ReturnValue", func(t *testing.T) {
		result := formatReqBody(&http.Request{}, []byte("yolo"))

		assert.Equal(t, "yolo", result)
	})
}

func Test_logSeverity(t *testing.T) {
	t.Run("POSITIVE-4xx-5xx_ReturnErrorLevel", func(t *testing.T) {
		for httpCode := 400; httpCode < 600; httpCode++ {
			result := logSeverity(httpCode)

			assert.Equal(t, zerolog.ErrorLevel, result)
		}

	})
	t.Run("POSITIVE-3xx_ReturnWarnLevel", func(t *testing.T) {
		for httpCode := 300; httpCode < 400; httpCode++ {
			result := logSeverity(httpCode)

			assert.Equal(t, zerolog.WarnLevel, result)
		}
	})
	t.Run("POSITIVE-2xx_ReturnWarnLevel", func(t *testing.T) {
		for httpCode := 200; httpCode < 300; httpCode++ {
			result := logSeverity(httpCode)

			assert.Equal(t, zerolog.InfoLevel, result)
		}
	})
	t.Run("POSITIVE-1xx_ReturnWarnLevel", func(t *testing.T) {
		for httpCode := 100; httpCode < 200; httpCode++ {
			result := logSeverity(httpCode)

			assert.Equal(t, zerolog.DebugLevel, result)
		}
	})
}
