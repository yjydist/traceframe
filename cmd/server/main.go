package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/config"
	"github.com/yjydist/traceframe/internal/httpapi"
	"github.com/yjydist/traceframe/internal/logging"
	"github.com/yjydist/traceframe/internal/models"
	openaiadapter "github.com/yjydist/traceframe/internal/models/openai"
	"github.com/yjydist/traceframe/internal/orchestrator"
	"github.com/yjydist/traceframe/internal/storage/sqlite"
	"github.com/yjydist/traceframe/internal/workflow"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := sqlite.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	repository := sqlite.NewRepository(db)
	projects := application.NewProjectService(repository)
	runtimeRepository := sqlite.NewRuntimeRepository(db)
	var modelClient models.ModelClient = models.UnconfiguredClient{}
	if cfg.ModelProvider == "openai" {
		modelClient = openaiadapter.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAIBaseURL, nil)
	}
	runs := orchestrator.NewService(projects, runtimeRepository, runtimeRepository, modelClient, logger)
	workflowService := workflow.NewService(projects, sqlite.NewWorkflowRepository(db))
	runs.Start(ctx)

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           httpapi.New(db, projects, runs, workflowService, cfg.WebDir, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server listening", "address", cfg.Address, "database_path", cfg.DatabasePath)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		logger.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
