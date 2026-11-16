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

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/cache"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/httpserver"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/logging"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/metrics"
	logsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/logs/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/providers"
	notifusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/observability"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/crypto"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/fieldcrypt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/queue"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/realtime"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/tracing"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(cfg.Log.Level, cfg.Log.Format)
	flushSentry, err := observability.Init(cfg.Sentry, "api")
	if err != nil {
		log.Warn("sentry_init_failed", "error", err)
	} else if observability.Enabled() {
		log.Info("sentry_enabled", "environment", cfg.Sentry.Environment, "traces_sample_rate", cfg.Sentry.TracesSampleRate)
	}
	defer flushSentry()
	// exit flushes pending error events before leaving (os.Exit skips defers).
	exit := func(code int) {
		flushSentry()
		os.Exit(code)
	}
	log = slog.New(observability.NewSlogHandler(log.Handler()))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// OpenTelemetry (#179): off unless OTEL_EXPORTER_OTLP_ENDPOINT is set.
	if shutdownTracing, err := tracing.Init(ctx, cfg.Tracing, "api"); err != nil {
		log.Warn("tracing_init_failed", "error", err)
	} else {
		if cfg.Tracing.Endpoint != "" {
			log.Info("tracing_enabled", "service", cfg.Tracing.ServiceName, "sample_ratio", cfg.Tracing.SampleRatio)
		}
		defer func() {
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdownTracing(sctx)
		}()
	}

	// Alan düzeyinde şifreleme (#168): ham banka/fatura verisi FIELD_ENCRYPTION_KEY
	// ile mühürlenir. Anahtar geçersizse başlama; düz metin yazmak sessizce olmasın.
	if fc, err := fieldcrypt.Install(cfg.Encryption); err != nil {
		log.Error("field_encryption_init_failed", "error", err)
		exit(1)
	} else if fc == nil {
		log.Warn("field_encryption_disabled")
	}
	db, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		log.Error("database_init_failed", "error", err)
		exit(1)
	}
	defer db.Close()

	rdb, err := cache.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		log.Error("redis_init_failed", "error", err)
		exit(1)
	}
	defer func() { _ = rdb.Close() }()

	store, err := storage.NewFromConfig(ctx, cfg.Storage)
	if err != nil {
		log.Error("storage_init_failed", "error", err)
		exit(1)
	}
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := store.Ping(pingCtx); err != nil {
		pingCancel()
		log.Error("storage_ping_failed", "error", err)
		exit(1)
	}
	pingCancel()

	var publisher realtime.Publisher
	if cfg.Centrifugo.Enabled {
		publisher = realtime.NewCentrifugoClient(cfg.Centrifugo)
	} else {
		publisher = realtime.NoopPublisher{}
	}

	queries := database.NewQueries(db)
	logsSvc := logsusecase.New(queries)
	persist := logging.Attach(log, logsSvc)
	// Outermost: the app_logs table and the error tracker get scrubbed
	// records too, with request and office ids (#179).
	log = slog.New(logging.Redact(persist.Logger().Handler()))
	defer persist.Close()

	eventBus := events.NewBus(log)
	mailer := mail.NewSMTPSender(cfg.SMTP)
	provs := []providers.Provider{
		providers.InappProvider{},
		providers.EmailProvider{Mail: mailer},
		providers.RealtimeProvider{Pub: publisher},
		providers.NoopProvider{Name: "sms", Log: log},
		providers.NoopProvider{Name: "push", Log: log},
	}

	var queueClient *queue.Client
	var worker *queue.Worker
	if cfg.Queue.Enabled {
		queueClient = queue.NewClient(cfg.Redis)
		secretBox, err := crypto.NewSecretBox(cfg.Encryption.Key)
		if err != nil {
			log.Error("encryption_init_failed", "error", err)
			exit(1)
		}
		notifSvc := notifusecase.New(queries, queueClient, provs, log).WithSecretBox(secretBox)
		if cfg.Queue.WorkerInProcess {
			// The in-process worker never takes fixed-egress tasks (issue
			// #45): they must leave from the egress worker's IP.
			wcfg := cfg
			wcfg.Queue.WorkerPool = queue.PoolGeneral
			worker = queue.NewWorker(wcfg, log, notifSvc.Deliver).
				WithLogPurge(logsSvc.ApplyDueRules)
			if n, err := notifSvc.ReclaimStuck(ctx, notifusecase.DefaultStuckProcessingMinutes); err != nil {
				log.Error("notification_reclaim_failed", "error", err)
			} else if n > 0 {
				log.Info("notification_reclaim_completed", "count", n)
			}
		}
	}

	srv, err := httpserver.New(cfg, log, httpserver.Deps{
		DB:       db,
		Queries:  queries,
		Redis:    rdb,
		Queue:    queueClient,
		Storage:  store,
		Realtime: publisher,
		Worker:   worker,
		Events:   eventBus,
	})
	if err != nil {
		log.Error("httpserver_init_failed", "error", err)
		exit(1)
	}

	// Prometheus metrics on their own port (#179).
	if err := metrics.Default.Serve(ctx, cfg.Metrics.Addr, cfg.Metrics.Token, log); err != nil {
		log.Warn("metrics_listen_failed", "addr", cfg.Metrics.Addr, "error", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown_signal_received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server_failed", "error", err)
			exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown_failed", "error", err)
		exit(1)
	}
	log.Info("server_stopped")
}
