package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	t.Run("POSITIVE-ValidEnv", func(t *testing.T) {
		os.Setenv("DATABASE_HOST", "localhost")
		os.Setenv("DATABASE_PORT", "5432")
		os.Setenv("DATABASE_USER", "user")
		os.Setenv("DATABASE_PASSWORD", "pass")
		os.Setenv("DATABASE_NAME", "dbname")
		os.Setenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
		os.Setenv("DATABASE_LOG_LEVEL", "info")
		cfg := Load()
		assert.Equal(t, "localhost", cfg.Host)
		assert.Equal(t, 5432, cfg.Port)
		assert.Equal(t, "user", cfg.User)
		assert.Equal(t, "pass", cfg.Password)
		assert.Equal(t, "dbname", cfg.Name)
		assert.NotEmpty(t, cfg.RabbitMQURL)
	})
}

func TestConfig_DataSourceName(t *testing.T) {
	t.Run("POSITIVE-Format", func(t *testing.T) {
		cfg := Config{
			User:     "user",
			Password: "pass",
			Host:     "localhost",
			Port:     5432,
			Name:     "dbname",
		}
		dsn := cfg.DataSourceName()
		assert.Equal(t, "user=user password=pass host=localhost port=5432 dbname=dbname sslmode=disable", dsn)
	})
}
