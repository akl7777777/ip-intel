package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	cfg := LoadConfig()

	svc := NewService(cfg)
	defer svc.Close()

	srv := NewServer(svc, cfg.AuthKey)

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
	log.Printf("[main] Local DB: %v | Known ASNs: %d | Providers: %d | Auth: %s",
		svc.localDB != nil, len(datacenterASNs), len(svc.providers), authStatus)

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[main] Server error: %v", err)
	}

	log.Println("[main] Server stopped")
}
