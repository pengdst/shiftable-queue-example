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

	cfg := Load()
	db := NewGORM(cfg)

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
	server := NewServer()
	server.Run(8080)
}

func runProcessor(cfg *Config, db *gorm.DB) {
	log.Info().Msg("Starting queue processor...")
	processor := NewQueueProcessor(cfg, db, &FakeAPI{})
	processor.Start()
}
