package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/VanceMichael/greengrid/internal/app"
	"github.com/VanceMichael/greengrid/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	runtime, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("create runtime", "error", err)
		os.Exit(1)
	}
	if err := runtime.Run(context.Background()); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
