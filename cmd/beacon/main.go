package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/magiconair/beacon/internal/migrate"
	"github.com/magiconair/beacon/internal/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	addr := envOr("ADDR", ":8080")
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	pub, err := parsePublicKey(os.Getenv("IDENTITY_PUBLIC_KEY_B64"))
	if err != nil {
		return fmt.Errorf("IDENTITY_PUBLIC_KEY_B64: %w", err)
	}
	keyID := os.Getenv("IDENTITY_KEY_ID")
	if keyID == "" {
		return errors.New("IDENTITY_KEY_ID is required")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	if err := migrate.Up(ctx, db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	cfg := server.Config{
		BaseURL:             envOr("BASE_URL", "http://localhost:8080"),
		SecureCookies:       envOr("SECURE_COOKIES", "false") == "true",
		WebAuthnRPID:        envOr("WEBAUTHN_RP_ID", "localhost"),
		WebAuthnRPName:      envOr("WEBAUTHN_RP_DISPLAY_NAME", "Beacon"),
		WebAuthnRPOrigins:   server.SplitCSV(envOr("WEBAUTHN_RP_ORIGINS", "http://localhost:8080,http://127.0.0.1:8080")),
		BootstrapAdminEmail: os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		BootstrapReissue:    os.Getenv("BOOTSTRAP_REISSUE") == "true",
		IdentityPublicKey:   pub,
		IdentityKeyID:       keyID,
	}
	srvApp, err := server.New(logger, db, cfg)
	if err != nil {
		return err
	}
	if err := srvApp.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srvApp.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", addr, "identity_key_id", keyID)
		errCh <- httpSrv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		logger.Info("shutdown signal", "signal", sig.String())
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func parsePublicKey(b64 string) ([32]byte, error) {
	var out [32]byte
	if b64 == "" {
		return out, errors.New("required")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return out, err
	}
	if len(raw) != 32 {
		return out, fmt.Errorf("want 32 bytes, got %d", len(raw))
	}
	copy(out[:], raw)
	return out, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
