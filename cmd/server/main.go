package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	authadapter "bestelltool_be/internal/adapters/auth"
	httpadapter "bestelltool_be/internal/adapters/http"
	"bestelltool_be/internal/adapters/postgres"
	"bestelltool_be/internal/adapters/sse"
	"bestelltool_be/internal/application/ports"
	"bestelltool_be/internal/application/usecases"
)

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}

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
	if cfg.RunMigrations {
		if err := postgres.RunEmbeddedMigrations(context.Background(), cfg.DatabaseURL); err != nil {
			return fmt.Errorf("run embedded migrations: %w", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create postgres pool: %w", err)
	}
	defer pool.Close()

	uow := postgres.NewUnitOfWork(pool)
	passwordHasher := authadapter.NewArgon2Hasher(authadapter.DefaultArgon2Config())
	secretGenerator := authadapter.NewSecretGenerator(authadapter.DefaultTokenConfig())
	clock := systemClock{}
	loginUseCase := usecases.NewLoginUseCase(uow, passwordHasher, secretGenerator, clock)
	tokenEncryptor, err := authadapter.NewTokenEncryptorFromBase64Key(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("create token encryptor: %w", err)
	}
	refreshSessionUseCase := usecases.NewRefreshSessionUseCase(uow, secretGenerator, tokenEncryptor, clock)
	logoutUseCase := usecases.NewLogoutUseCase(uow, clock)
	changeOwnPasswordUseCase := usecases.NewChangeOwnPasswordUseCase(uow, passwordHasher, clock)
	switchActiveRoleUseCase := usecases.NewSwitchActiveRoleUseCase(uow, secretGenerator, clock)
	getMeUseCase := usecases.NewGetMeUseCase(uow)
	var authenticator ports.Authenticator
	if cfg.AuthMode == "static" {
		authenticator, err = authadapter.ParseStaticTokens(cfg.StaticTokens)
		if err != nil {
			return fmt.Errorf("parse static tokens: %w", err)
		}
	} else {
		authenticator = authadapter.NewSessionAuthenticator(uow, postgres.NewPermissionResolver(pool), clock, cfg.PrincipalCacheTTL)
	}
	requestRepo := postgres.NewRequestRepository(pool)
	eventStream := sse.NewBroker(0)

	handler := httpadapter.NewHandlerWithEventStreamAndAuthenticationAndSecurity(
		authenticator,
		usecases.NewCreateRequestUseCaseWithPublisher(uow, eventStream),
		usecases.NewGetRequestUseCase(requestRepo),
		usecases.NewRequestReturnUseCaseWithPublisher(uow, eventStream),
		usecases.NewTransferResourceUseCaseWithPublisher(uow, eventStream),
		eventStream,
		loginUseCase,
		refreshSessionUseCase,
		logoutUseCase,
		changeOwnPasswordUseCase,
		httpadapter.NewRateLimiter(10, time.Minute, cfg.TrustProxyHeaders, time.Now),
		switchActiveRoleUseCase,
		getMeUseCase,
	)

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// SEC-06: log only the resolved config shape, never a secret. AUTH_STATIC_TOKENS
	// and any DATABASE_URL credential must never reach this call, by omission -
	// not by masking a value that was already collected here.
	slog.Info("server starting",
		"app_env", cfg.AppEnv,
		"http_addr", cfg.HTTPAddr,
		"run_migrations", cfg.RunMigrations,
	)

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
