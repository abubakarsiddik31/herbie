package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/httpapi"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: httpapi.NewServer(httpapi.ServerDeps{Cfg: cfg, Log: log})}
	log.Info("listening", "port", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}
