package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"goseq/web"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	storage, err := NewStorage(cfg.JournalsDir, cfg.Location)
	if err != nil {
		log.Fatalf("Storage initialization error: %v", err)
	}

	auth := NewAuthManager(cfg)

	tmpls, err := web.NewTemplates()
	if err != nil {
		log.Fatalf("Template initialization error: %v", err)
	}

	srv := NewServer(cfg, storage, auth, tmpls)

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("GoSeq server listening on http://%s", addr)
		log.Printf("Journals directory: %s", cfg.JournalsDir)
		log.Printf("Timezone locked to: %s (%s)", cfg.Location.String(), time.Now().In(cfg.Location).Format("2006-01-02 15:04:05 MST"))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down GoSeq server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("GoSeq server exited cleanly.")
}
