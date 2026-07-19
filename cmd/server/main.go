package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	authadapter "bestelltool_be/internal/adapters/auth"
	httpadapter "bestelltool_be/internal/adapters/http"
	"bestelltool_be/internal/adapters/postgres"
	"bestelltool_be/internal/application/usecases"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("server stopped with error: %v", err)
	}
}

func run() error {
	cfg, err := loadConfigFromEnv()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create postgres pool: %w", err)
	}
	defer pool.Close()

	authenticator, err := authadapter.ParseStaticTokens(cfg.StaticTokens)
	if err != nil {
		return fmt.Errorf("parse static tokens: %w", err)
	}

	uow := postgres.NewUnitOfWork(pool)
	requestRepo := postgres.NewRequestRepository(pool)

	handler := httpadapter.NewHandler(
		authenticator,
		usecases.NewCreateRequestUseCase(uow),
		usecases.NewGetRequestUseCase(requestRepo),
		usecases.NewRequestReturnUseCase(uow),
	)

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("listen and serve: %w", err)
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	if err, ok := <-serveErr; ok && err != nil {
		return err
	}

	return nil
}
