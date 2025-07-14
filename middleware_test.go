package main

import (
	"bytes"
	"encoding/json"
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

	t.Run("POSITIVE-WithXForwardedFor", func(t *testing.T) {
		h := realIPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "1.2.3.4", r.RemoteAddr)
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POSITIVE-WithTrueClientIP", func(t *testing.T) {
		h := realIPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "1.2.3.4", r.RemoteAddr)
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("True-Client-IP", "1.2.3.4")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("NEGATIVE-InvalidIP", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		initialRemoteAddr := req.RemoteAddr
		h := realIPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, initialRemoteAddr, r.RemoteAddr)
			w.WriteHeader(http.StatusOK)
		}))
		req.Header.Set("X-Real-IP", "invalid-ip")
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

func Test_chainMiddleware(t *testing.T) {
	t.Run("POSITIVE-ChainMiddlewares_ExecuteInOrder", func(t *testing.T) {
		var result []string
		mw1 := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				result = append(result, "mw1-before")
				h.ServeHTTP(w, r)
				result = append(result, "mw1-after")
			})
		}
		mw2 := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				result = append(result, "mw2-before")
				h.ServeHTTP(w, r)
				result = append(result, "mw2-after")
			})
		}
		finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			result = append(result, "handler")
			w.WriteHeader(http.StatusOK)
		})

		chained := chainMiddleware(finalHandler, mw1, mw2)
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		chained.ServeHTTP(w, req)

		expected := []string{"mw2-before", "mw1-before", "handler", "mw1-after", "mw2-after"}
		assert.Equal(t, expected, result)
	})
}

func Test_logFields_MarshalZerologObject(t *testing.T) {
	t.Run("POSITIVE-Marshal_ReturnsCorrectJSON", func(t *testing.T) {
		fields := &logFields{
			RemoteIP:   "1.1.1.1",
			Host:       "example.com",
			UserAgent:  "go-test",
			Method:     "GET",
			Path:       "/",
			Body:       `{"a":1}`,
			StatusCode: 200,
			Latency:    123.45,
		}

		// Use zerolog's test hook to capture output
		var buf bytes.Buffer
		logger := zerolog.New(&buf)
		logger.Info().EmbedObject(fields).Msg("")

		var logged map[string]any
		err := json.Unmarshal(buf.Bytes(), &logged)
		assert.NoError(t, err)

		assert.Equal(t, "1.1.1.1", logged["remote_ip"])
		assert.Equal(t, "example.com", logged["host"])
		assert.Equal(t, "go-test", logged["user_agent"])
		assert.Equal(t, "GET", logged["method"])
		assert.Equal(t, "/", logged["path"])
		assert.Equal(t, `{"a":1}`, logged["body"])
		// Note: zerolog marshals numbers as float64
		assert.Equal(t, float64(200), logged["status_code"])
		assert.Equal(t, 123.45, logged["latency"])
	})
}