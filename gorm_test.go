package main

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestNewLogLevel(t *testing.T) {
	t.Run("POSITIVE-InfoLevel", func(t *testing.T) {
		l := NewLogLevel("info")
		assert.Equal(t, zerolog.InfoLevel, l.Level)
	})
	t.Run("POSITIVE-WarnLevel", func(t *testing.T) {
		l := NewLogLevel("warn")
		assert.Equal(t, zerolog.WarnLevel, l.Level)
	})
	t.Run("POSITIVE-DebugLevel", func(t *testing.T) {
		l := NewLogLevel("debug")
		assert.Equal(t, zerolog.DebugLevel, l.Level)
	})
	t.Run("POSITIVE-ErrorLevel", func(t *testing.T) {
		l := NewLogLevel("error")
		assert.Equal(t, zerolog.ErrorLevel, l.Level)
	})
	t.Run("POSITIVE-DefaultLevel", func(t *testing.T) {
		l := NewLogLevel("unknown")
		assert.Equal(t, zerolog.Disabled, l.Level)
	})
}

func TestLogger_logWithCtx(t *testing.T) {
	t.Run("POSITIVE-WithContext", func(t *testing.T) {
		l := NewLogLevel("info")
		ctx := context.Background()
		evt := l.logWithCtx(ctx, zerolog.InfoLevel)
		assert.NotNil(t, evt)
	})
	t.Run("POSITIVE-WithoutContext", func(t *testing.T) {
		l := NewLogLevel("info")
		evt := l.logWithCtx(nil, zerolog.InfoLevel)
		assert.NotNil(t, evt)
	})
}

func TestLogger_Trace(t *testing.T) {
	t.Run("POSITIVE-TraceNoError", func(t *testing.T) {
		l := NewLogLevel("info")
		begin := time.Now()
		f := func() (string, int64) { return "SELECT 1", 1 }
		l.Trace(context.Background(), begin, f, nil)
		// no panic, no assert needed
	})

	t.Run("POSITIVE-TraceWithError", func(t *testing.T) {
		l := NewLogLevel("info")
		begin := time.Now()
		f := func() (string, int64) { return "SELECT 1", 1 }
		err := context.DeadlineExceeded
		l.Trace(context.Background(), begin, f, err)
		// no panic, no assert needed
	})
}
