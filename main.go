package main

import (
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal"
	"github.com/syrilster/migrate-leave-krow-to-xero/internal/config"
)

// init is invoked before main()
func init() {
	// loads values from .env into the system
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}
}

func main() {
	cfg := config.NewApplicationConfig()
	server := internal.SetupServer(cfg)
	server.Start("", cfg.ServerPort())
}
