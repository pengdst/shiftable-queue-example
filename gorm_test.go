package main

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	t.Run("POSITIVE-WithZerologContext", func(t *testing.T) {
		l := NewLogLevel("info")
		ctx := log.With().Logger().WithContext(context.Background())
		evt := l.logWithCtx(ctx, zerolog.InfoLevel)
		assert.NotNil(t, evt)
	})
	t.Run("POSITIVE-WithoutContext", func(t *testing.T) {
		l := NewLogLevel("info")
		evt := l.logWithCtx(nil, zerolog.InfoLevel)
		assert.NotNil(t, evt)
	})
	t.Run("POSITIVE-CtxBackground_ReturnNil", func(t *testing.T) {
		l := NewLogLevel("info")
		ctx := context.Background()
		evt := l.logWithCtx(ctx, zerolog.InfoLevel)
		assert.Nil(t, evt)
	})
}

func TestLogger_safeLogMsgf(t *testing.T) {
	t.Run("POSITIVE-WithZerologContext", func(t *testing.T) {
		l := NewLogLevel("info")
		ctx := log.With().Logger().WithContext(context.Background())
		l.logMsgf(ctx, "test")
	})
	t.Run("POSITIVE-WithoutContext", func(t *testing.T) {
		l := NewLogLevel("info")
		l.logMsgf(nil, "test")
	})
	t.Run("POSITIVE-CtxBackground_NoError", func(t *testing.T) {
		l := NewLogLevel("info")
		ctx := context.Background()
		l.logMsgf(ctx, "test")
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

func TestLogger_LogMode(t *testing.T) {
	l := NewLogLevel("info")
	ret := l.LogMode(0)
	assert.Equal(t, l, ret)
}

func TestLogger_Error(t *testing.T) {
	t.Run("POSITIVE-WithZerologContext", func(t *testing.T) {
		l := NewLogLevel("info")
		ctx := log.With().Logger().WithContext(context.Background())
		l.Error(ctx, "error message")
	})
	t.Run("POSITIVE-WithNilContext", func(t *testing.T) {
		l := NewLogLevel("info")
		l.Error(nil, "error message")
	})
	t.Run("POSITIVE-WithBackgroundContext", func(t *testing.T) {
		l := NewLogLevel("info")
		l.Error(context.Background(), "error message")
	})
}

func TestLogger_Warn(t *testing.T) {
	l := NewLogLevel("warn")
	t.Run("POSITIVE-WithZerologContext", func(t *testing.T) {
		ctx := log.With().Logger().WithContext(context.Background())
		l.Warn(ctx, "warn message")
	})
	t.Run("POSITIVE-WithNilContext", func(t *testing.T) {
		l.Warn(nil, "warn message")
	})
	t.Run("POSITIVE-WithBackgroundContext", func(t *testing.T) {
		l.Warn(context.Background(), "warn message")
	})
}

func TestLogger_Info(t *testing.T) {
	t.Run("POSITIVE-WithZerologContext", func(t *testing.T) {
		l := NewLogLevel("info")
		ctx := log.With().Logger().WithContext(context.Background())
		l.Info(ctx, "info message")
	})
	t.Run("POSITIVE-WithNilContext", func(t *testing.T) {
		l := NewLogLevel("info")
		l.Info(nil, "info message")
	})
	t.Run("POSITIVE-WithBackgroundContext", func(t *testing.T) {
		l := NewLogLevel("info")
		l.Info(context.Background(), "info message")
	})
}

func TestNewGORM(t *testing.T) {
	t.Run("NEGATIVE-InvalidDataSourceName", func(t *testing.T) {
		cfg := &Config{
			Host:     "invalid",
			Port:     1,
			User:     "invalid",
			Password: "invalid",
			Name:     "invalid",
			LogLevel: "info",
		}
		db, err := NewGORM(cfg)
		assert.Error(t, err)
		assert.Nil(t, db)
	})
}
