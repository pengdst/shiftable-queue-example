package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
