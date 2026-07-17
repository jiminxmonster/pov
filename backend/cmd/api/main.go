package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jiminxmonster/pov/backend/internal/app"
)

func main() {
	config := app.ConfigFromEnv()
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthcheck(config.Port)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server, err := app.New(ctx, config)
	if err != nil {
		log.Fatalf("start api: %v", err)
	}
	defer server.Close()
	server.StartPublicDataSync(ctx)

	httpServer := &http.Server{
		Addr:              ":" + config.Port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownContext)
	}()

	log.Printf("POV API listening on :%s", config.Port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve api: %v", err)
	}
}

func healthcheck(port string) {
	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
	if err != nil || response.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	_ = response.Body.Close()
}
