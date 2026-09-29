package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/magiconair/beacon/internal/migrate"
	"github.com/magiconair/beacon/internal/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger, os.Args[1:]); err != nil {
		logger.Error("command failed", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: beacon <server|wipe-schema>")
	}
	switch args[0] {
	case "server":
		return runServer(logger)
	case "wipe-schema":
		return runWipeSchema(logger)
	default:
		return fmt.Errorf("unknown command %q (want server or wipe-schema)", args[0])
	}
}

func runWipeSchema(logger *slog.Logger) error {
	if os.Getenv("BEACON_ALLOW_SCHEMA_WIPE") != "true" {
		return errors.New("wipe-schema requires BEACON_ALLOW_SCHEMA_WIPE=true")
	}
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	// Single connection: avoid the pool racing terminate_backend against ourselves.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	logger.Warn("wiping public schema")
	if err := migrate.WipePublicSchema(ctx, db); err != nil {
		return fmt.Errorf("wipe schema: %w", err)
	}
	logger.Info("public schema wiped")
	return nil
}

func runServer(logger *slog.Logger) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

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
		MaxReceiptBytes:     5 << 20,
		ArrivalFutureDays:   envInt("ARRIVAL_FUTURE_DAYS", 1),
	}
	srvApp, err := server.New(logger, db, cfg)
	if err != nil {
		return err
	}
	if err := srvApp.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	httpSrv := &http.Server{
		Addr:              listenAddr(),
		Handler:           srvApp.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", httpSrv.Addr)
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

func openDB() (*sql.DB, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}

// listenAddr prefers ADDR; otherwise PORT (Railway/PaaS); default :8080.
func listenAddr() string {
	if v := os.Getenv("ADDR"); v != "" {
		return v
	}
	if p := os.Getenv("PORT"); p != "" {
		if strings.HasPrefix(p, ":") {
			return p
		}
		return ":" + p
	}
	return ":8080"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
