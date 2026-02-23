package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/akl7777777/ip-intel/internal/config"
	"github.com/akl7777777/ip-intel/internal/lookup"
	"github.com/akl7777777/ip-intel/internal/server"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	cfg := config.Load()

	svc := lookup.NewService(cfg)
	defer svc.Close()

	srv := server.New(svc, cfg.AuthKey)

	addr := cfg.Host + ":" + cfg.Port
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("[main] Shutting down...")
		httpServer.Close()
	}()

	authStatus := "disabled"
	if cfg.AuthKey != "" {
		authStatus = "enabled"
	}
	log.Printf("[main] IP Intel service starting on %s", addr)
	log.Printf("[main] Auth: %s", authStatus)

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[main] Server error: %v", err)
	}

	log.Println("[main] Server stopped")
}
