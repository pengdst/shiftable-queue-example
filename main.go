package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func main() {
	var command string
	flag.StringVar(&command, "cmd", "server", "Command to run: server, processor")
	flag.Parse()

	cfg, err := Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}
	db, err := NewGORM(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create GORM instance")
	}

	switch command {
	case "api":
		runServer()
	case "queue":
		runProcessor(cfg, db)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Available commands: server, processor")
		os.Exit(1)
	}
}

func runServer() {
	log.Info().Msg("Starting HTTP server...")
	server, err := NewServer()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create server")
	}

	if err := server.Run(8080); err != nil {
		log.Fatal().Err(err).Msg("failed to run server")
	}
}

func runProcessor(cfg *Config, db *gorm.DB) {
	log.Info().Msg("Starting queue processor...")
	processor, err := NewQueueProcessor(cfg, db, &FakeAPI{})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create queue processor")
	}

	if err := processor.Start(); err != nil {
		log.Fatal().Err(err).Msg("failed to start queue processor")
	}
}
