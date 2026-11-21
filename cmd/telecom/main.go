package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"telecom/internal/service/admin"
	"time"

	"telecom/internal/config"
	"telecom/internal/lib/logger/slog/slogpretty"
	"telecom/internal/repository/postgres"
	"telecom/internal/service/auth"
	"telecom/internal/service/cabinet"
	"telecom/internal/service/finance"
)

func main() {
	const op = "2d2.cmd.2d2.main"

	cfg := config.MustLoad()

	logger := slogpretty.SetupPrettyLogger()
	log := logger.With(slog.String("op", op))
	log.Info("logger is created")

	repository, err := postgres.NewRepository(cfg)
	if err != nil {
		log.Error("database init failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer repository.Close()
	log.Info("database is loaded")

	services := Services{
		auth:    auth.NewService(repository, cfg),
		cabinet: cabinet.NewService(repository),
		finance: finance.NewService(repository, cfg),
		admin:   admin.NewService(repository),
	}
	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:              address,
		Handler:           createRouter(logger, cfg, services),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	go runBilling(ctx, repository, log)

	go func() {
		log.Info("server is started on " + address)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server failed", slog.String("error", err.Error()))
			stop()
		}
	}()

	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		log.Error("server shutdown failed", slog.String("error", err.Error()))
	}
}

func runBilling(ctx context.Context, rep *postgres.Repository, log *slog.Logger) {
	// Internal process, no extra public HTTP endpoint or external scheduler.
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		loc, _ := time.LoadLocation("Europe/Minsk")
		now := time.Now().In(loc)
		current := now.Format("2006-01")
		previous := now.AddDate(0, -1, 0).Format("2006-01")
		jobCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		if err := rep.RunBilling(jobCtx, previous); err != nil {
			log.Error("previous month billing failed", slog.String("error", err.Error()))
		}
		if err := rep.CloseBillingMonth(jobCtx, previous); err != nil {
			log.Error("closing previous month failed", slog.String("error", err.Error()))
		}
		if err := rep.RunBilling(jobCtx, current); err != nil {
			log.Error("current month billing failed", slog.String("error", err.Error()))
		}
		if err := rep.CreateAlerts(jobCtx, current); err != nil {
			log.Error("monitoring failed", slog.String("error", err.Error()))
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
