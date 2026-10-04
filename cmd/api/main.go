package main

import (
	"context"
	"log"
	"net/http"

	"github.com/amishbpatel/clusterstor/internal/config"
	"github.com/amishbpatel/clusterstor/internal/database"
	"github.com/amishbpatel/clusterstor/internal/httpapi"
)

func main() {
	cfg := config.Load()

	pool, err := database.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(pool),
	}

	log.Printf("clusterstor api listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
