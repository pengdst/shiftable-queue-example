package main

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/rs/zerolog/log"
)

func Load() *Config {
	var c Config
	if err := env.Parse(&c); err != nil {
		log.Fatal().Msgf("unable to parse env: %s", err.Error())
	}

	return &c
}

type Config struct {
	Host     string `env:"DATABASE_HOST,notEmpty"`
	Port     int    `env:"DATABASE_PORT,notEmpty"`
	User     string `env:"DATABASE_USER,notEmpty"`
	Password string `env:"DATABASE_PASSWORD,notEmpty"`
	Name     string `env:"DATABASE_NAME,notEmpty"`
	LogLevel string `env:"DATABASE_LOG_LEVEL" envDefault:"info"`
}

func (c Config) DataSourceName() string {
	return fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s sslmode=disable",
		c.User, c.Password, c.Host, c.Port, c.Name)
}
