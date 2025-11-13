package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bark-labs/bark-secure-proxy/internal/barkclient"
	"github.com/bark-labs/bark-secure-proxy/internal/config"
	"github.com/bark-labs/bark-secure-proxy/internal/server"
	"github.com/bark-labs/bark-secure-proxy/internal/service"
	"github.com/bark-labs/bark-secure-proxy/internal/storage/bolt"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	barkClient, err := barkclient.New(cfg.Bark.BaseURL, cfg.Bark.Token, cfg.Bark.RequestTimeout)
	if err != nil {
		log.Fatalf("init bark client: %v", err)
	}

	store, err := bolt.New(cfg.Storage.Path)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	authSvc := service.NewAuthService(cfg)
	deviceSvc := service.NewDeviceService(store, cfg, barkClient)
	noticeSvc := service.NewNoticeService(store, barkClient)
	logSvc := service.NewNoticeLogService(store, deviceSvc)

	srv := server.New(cfg, store, deviceSvc, noticeSvc, logSvc, authSvc, barkClient)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("server stopped: %v", err)
		}
	}()

	// graceful shutdown
	waitForSignal()
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.WriteTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func waitForSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
