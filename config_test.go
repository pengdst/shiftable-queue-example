package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// setEnv is a helper function to set environment variables for a test and clean them up afterwards.
func setEnv(t *testing.T, envs map[string]string) {
	for key, value := range envs {
		os.Setenv(key, value)
	}

	t.Cleanup(func() {
		for key, _ := range envs {
			os.Unsetenv(key)
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("POSITIVE-ValidEnv", func(t *testing.T) {
		setEnv(t, map[string]string{
			"DATABASE_HOST":      "localhost",
			"DATABASE_PORT":      "5432",
			"DATABASE_USER":      "user",
			"DATABASE_PASSWORD":  "pass",
			"DATABASE_NAME":      "dbname",
			"RABBITMQ_URL":       "amqp://guest:guest@localhost:5672/",
			"DATABASE_LOG_LEVEL": "info",
		})
		cfg, err := Load()
		assert.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "localhost", cfg.Host)
		assert.Equal(t, 5432, cfg.Port)
		assert.Equal(t, "user", cfg.User)
		assert.Equal(t, "pass", cfg.Password)
		assert.Equal(t, "dbname", cfg.Name)
		assert.NotEmpty(t, cfg.RabbitMQURL)
		assert.Equal(t, "info", cfg.LogLevel)
	})

	t.Run("POSITIVE-DefaultLogLevel", func(t *testing.T) {
		setEnv(t, map[string]string{
			"DATABASE_HOST":     "localhost",
			"DATABASE_PORT":     "5432",
			"DATABASE_USER":     "user",
			"DATABASE_PASSWORD": "pass",
			"DATABASE_NAME":     "dbname",
			"RABBITMQ_URL":      "amqp://guest:guest@localhost:5672/",
			// DATABASE_LOG_LEVEL is intentionally not set to test default
		})

		cfg, err := Load()
		assert.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "info", cfg.LogLevel)
	})

	t.Run("NEGATIVE-MissingRequiredEnv", func(t *testing.T) {
		setEnv(t, map[string]string{
			"DATABASE_HOST":      "",
			"DATABASE_PORT":      "",
			"DATABASE_USER":      "",
			"DATABASE_PASSWORD":  "",
			"DATABASE_NAME":      "",
			"RABBITMQ_URL":       "",
			"DATABASE_LOG_LEVEL": "",
		})

		cfg, err := Load()
		assert.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "should not be empty")
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
