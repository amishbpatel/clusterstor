package main

import (
	"context"
	"log"
	"net/http"

	"github.com/amishbpatel/clusterstor/internal/config"
	"github.com/amishbpatel/clusterstor/internal/database"
	"github.com/amishbpatel/clusterstor/internal/httpapi"
	"github.com/amishbpatel/clusterstor/internal/providers"
)

func main() {
	cfg := config.Load()

	pool, err := database.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	providerService, err := providers.NewService(pool, providers.Config{
		APIBaseURL: cfg.APIBaseURL,
		PublicBaseURL: cfg.PublicBaseURL,
		GoogleClientID: cfg.GoogleClientID,
		GoogleClientSecret: cfg.GoogleClientSecret,
		TokenEncryptionKey: cfg.TokenEncryptionKey,
	})
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(pool, providerService),
	}

	log.Printf("clusterstor api listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
