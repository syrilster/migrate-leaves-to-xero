package main

import (
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/config"
)

func main() {
	// load values from .env into the system
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}

	cfg, err := config.NewApplicationConfig()
	if err != nil {
		log.Fatalf("failed to start application: %v", err)
	}
	server := internal.SetupServer(cfg)
	server.Start("", cfg.ServerPort())
}
