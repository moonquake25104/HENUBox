// Kairos Go 后端入口：加载配置、连接数据库、执行迁移、启动 chi HTTP 服务。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kairos/server/internal/config"
	"kairos/server/internal/domain/ai"
	"kairos/server/internal/domain/notify"
	"kairos/server/internal/domain/ntfy"
	"kairos/server/internal/domain/push"
	"kairos/server/internal/domain/retention"
	"kairos/server/internal/httpapi"
	"kairos/server/internal/httpapi/handlers"
	"kairos/server/internal/schoolconn"
	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)

	if err := config.LoadEnv(".env", "../.env"); err != nil {
		log.Error("load .env", "error", err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DBURL)
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := migrate.Up(ctx, pool); err != nil {
		log.Error("run migrations", "error", err)
		os.Exit(1)
	}
	log.Info("migrations applied")

	q := store.New(pool)

	// Phase C direct connection: enabled only when a key file is explicitly
	// configured; the interactive login additionally requires the
	// local-mode flag, a discoverable Edge and the helper project.
	var henuConnector *handlers.HenuConnector
	if keyFile := os.Getenv("XIQUE_HENU_KEY_FILE"); keyFile != "" {
		helperDir := os.Getenv("XIQUE_HENU_HELPER_DIR")
		if helperDir == "" {
			helperDir = "../connectors/henu-browser"
		}
		profileRoot := os.Getenv("XIQUE_HENU_PROFILE_ROOT")
		if profileRoot == "" {
			profileRoot = filepath.Join(cfg.DataDir, "henu-login-profiles")
		}
		stderrDir := os.Getenv("XIQUE_HENU_LOGIN_LOG_DIR")
		if stderrDir == "" {
			stderrDir = filepath.Join(cfg.DataDir, "henu-login-logs")
		}
		henuConnector = &handlers.HenuConnector{
			BaseURL:          schoolconn.ProductionBaseURL,
			KeyFile:          keyFile,
			LocalInteractive: os.Getenv("XIQUE_HENU_LOCAL") == "1" && schoolconn.FindEdge() != "",
			Helper: &schoolconn.LoginManager{
				ScriptPath:  filepath.Join(helperDir, "helper.mjs"),
				EdgePath:    schoolconn.FindEdge(),
				LoginURL:    schoolconn.ProductionBaseURL + "/cas/login.action",
				Origin:      schoolconn.ProductionBaseURL,
				ProfileRoot: profileRoot,
				StderrDir:   stderrDir,
				Log:         log,
			},
			NewClient: func() (*schoolconn.Client, error) {
				return schoolconn.NewClient(schoolconn.ProductionBaseURL)
			},
			Log: log,
		}
		log.Info("henu direct connection",
			"local_interactive", henuConnector.LocalInteractive,
			"helper_available", henuConnector.Helper.Available())
	}

	nativePush, err := ntfy.Load(os.Getenv("XIQUE_NTFY_CONFIG"))
	if err != nil {
		log.Error("load private ntfy configuration")
		os.Exit(1)
	}
	pushKeys, err := push.LoadKeys(cfg.DataDir)
	if err != nil {
		log.Error("load push keys")
		os.Exit(1)
	}
	pushService := push.New(q, pushKeys, log)
	go pushService.Run(ctx)
	// Expired previews and their raw uploads are reclaimed at startup and then
	// on a fixed interval; a failed file delete stays queued across restarts.
	cleaner := &retention.Cleaner{Q: q, Root: cfg.DataDir + "/academic/previews", Log: log}
	go cleaner.Run(ctx, 15*time.Minute)
	generator := ai.NewGenerator(q, cfg.DataDir)
	mailer := notify.NewMailer(notify.SMTPConfig{
		Host: cfg.SMTPHost,
		Port: cfg.SMTPPort,
		User: cfg.SMTPUser,
		Pass: cfg.SMTPPass,
		From: cfg.SMTPFrom,
		To:   cfg.SMTPTo,
		TLS:  cfg.SMTPTLS,
	}, log)
	scheduler := notify.NewScheduler(q, mailer, generator, log)

	handler := httpapi.New(httpapi.Options{
		NativePush:   nativePush,
		Push:         pushService,
		Log:          log,
		Username:     cfg.Username,
		PasswordHash: cfg.PasswordHash,
		JWTSecret:    []byte(cfg.JWTSecret),
		JWTTTL:       cfg.JWTTTL,
		DB:           pool,
		Store:        q,
		Pool:         pool,
		DataDir:      cfg.DataDir,
		AIGenerator:  generator,

		NotifyScheduler:  scheduler,
		PomodoroNotifier: scheduler,
		Henu:             henuConnector,
		// Exact-match CORS allowlist for the mobile app origin (e.g.
		// "https://localhost"); unset keeps the desktop behavior unchanged.
		CORSAllowedOrigins: strings.Split(
			strings.TrimSpace(os.Getenv("KAIROS_CORS_ALLOW_ORIGINS")), ","),
	})

	scheduler.Start(ctx)
	log.Info("notifications", "smtp_enabled", mailer.Enabled())

	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.BindHost, cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("kairos-api listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server stopped", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown", "error", err)
	}
}
