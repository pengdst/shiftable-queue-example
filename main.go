package main

import (
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	cfg := Load()
	db := NewGORM(cfg)
	server := NewServer(db)
	server.Run(8080)
}
