package main

import (
	"log"
	"net/http"

	"github.com/amishbpatel/clusterstor/internal/config"
	"github.com/amishbpatel/clusterstor/internal/httpapi"
)

func main() {
	cfg := config.Load()
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.NewRouter()}
	log.Printf("clusterstor api listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
