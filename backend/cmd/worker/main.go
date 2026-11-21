package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/cache"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/logging"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/metrics"
	accountrulesmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules"
	accountrulesrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/repository"
	accountrulesusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/usecase"
	agentsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agents/usecase"
	bulkusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/bulk/usecase"
	doctasksmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/entegrator"
	exportusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/exports/usecase"
	importusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/imports/usecase"
	invoiceupload "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/upload"
	logsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/logs/usecase"
	notifmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/providers"
	notifusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/officedash"
	reconciliationmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reconciliation"
	reportsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reports"
	reviewrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources/easytrade"
	vatnotice "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/notice"
	vatreportrules "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/reportrules"
	vatforecastrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/repository"
	vatforecastusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	vatwatch "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/watch"
	warningsconnections "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/connections"
	warningsengine "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/observability"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/bulkengine"
	bulkadapters "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/bulkengine/adapters"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/crypto"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/egress"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/fieldcrypt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
	ioadapters "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine/adapters"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/outbox"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/searchengine"
	searchadapters "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/searchengine/adapters"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/queue"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/realtime"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/tracing"

	"github.com/hibiken/asynq"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(cfg.Log.Level, cfg.Log.Format)
	flushSentry, err := observability.Init(cfg.Sentry, "worker")
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
	if !cfg.Queue.Enabled {
		log.Error("queue_disabled")
		exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// OpenTelemetry (#179): off unless OTEL_EXPORTER_OTLP_ENDPOINT is set.
	if shutdownTracing, err := tracing.Init(ctx, cfg.Tracing, "worker"); err != nil {
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
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		log.Error("database_init_failed", "error", err)
		exit(1)
	}
	defer pool.Close()

	store, err := storage.NewFromConfig(ctx, cfg.Storage)
	if err != nil {
		log.Error("storage_init_failed", "error", err)
		exit(1)
	}

	var publisher realtime.Publisher
	if cfg.Centrifugo.Enabled {
		publisher = realtime.NewCentrifugoClient(cfg.Centrifugo)
	} else {
		publisher = realtime.NoopPublisher{}
	}

	queries := database.NewQueries(pool)
	mailer := mail.NewSMTPSender(cfg.SMTP)
	provs := []providers.Provider{
		providers.InappProvider{},
		providers.NewEmailProvider(mailer, cfg.Auth.FrontendURL, cfg.Encryption.Key),
		providers.RealtimeProvider{Pub: publisher},
		providers.NoopProvider{Name: "sms", Log: log},
		providers.NoopProvider{Name: "push", Log: log},
	}
	secretBox, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		log.Error("encryption_init_failed", "error", err)
		exit(1)
	}
	notifSvc := notifusecase.New(queries, nil, provs, log).WithSecretBox(secretBox)
	notifSvc.WithVAPID(notifusecase.VAPIDConfig{
		PublicKey:  cfg.VAPID.PublicKey,
		PrivateKey: cfg.VAPID.PrivateKey,
		Subject:    cfg.VAPID.Subject,
	})
	notifSvc.WithExpo(notifusecase.ExpoConfig{AccessToken: cfg.Expo.AccessToken})

	eventBus := events.NewBus(log)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log)
	outboxStore := outbox.NewStore(pool, queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log)
	outboxStop := outboxPub.StartRun(ctx)
	defer outboxStop()

	activityRec := activity.NewRecorder(queries, log)
	ioReg := ioengine.NewRegistry(
		ioadapters.NewUsers(queries),
		ioadapters.NewRoles(queries),
		ioadapters.NewNotifications(queries),
		ioadapters.NewActivity(queries),
	)
	exportSvc := exportusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	importSvc := importusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(queries),
		bulkadapters.NewRoles(queries),
	)
	bulkSvc := bulkusecase.New(queries, bulkReg, nil, notifSvc, activityRec, cfg.Bulk, log)
	logsSvc := logsusecase.New(queries)
	searchReg := searchengine.NewRegistry(
		searchadapters.NewUsers(queries),
		searchadapters.NewRoles(queries),
	)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, nil, log)
	// Deliveries run inline here; mobile pushes go through the queue (retries).
	queueClient := queue.NewClient(cfg.Redis)
	defer func() { _ = queueClient.Close() }()
	notifSvc.WithPushQueue(queueClient)
	persist := logging.Attach(log, logsSvc)
	// Outermost: the app_logs table and the error tracker get scrubbed
	// records too, with request and office ids (#179).
	log = slog.New(logging.Redact(persist.Logger().Handler()))
	defer persist.Close()

	// Entegratör senkronu (#40): bağlantı kimlik bilgileri sır kasasında.
	// Anahtar halkası geçersizse diğer işler sürer; senkron ErrNoCredentials verir.
	var vault secrets.Vault
	if keyring, err := secrets.ParseKeyring(cfg.Encryption.SecretsKeyring, cfg.Encryption.SecretsActiveKeyID, cfg.Encryption.Key); err != nil {
		log.Warn("secrets_keyring_invalid", "error", err)
	} else {
		vault = secrets.New(keyring, secrets.NewPGStore(db.New(pool)))
	}
	// Sabit çıkış IP'si (#45): entegratör çağrıları fixed-egress havuzunda,
	// EGRESS_PROXY_URL varsa o vekil üzerinden çıkar. IP denetimi de aynı
	// istemciyi kullanır, böylece denetlenen yol entegratörün gördüğü yoldur.
	egressCfg := egress.Config{
		ProxyURL: cfg.Egress.ProxyURL, ExpectedIPs: cfg.Egress.ExpectedIPs,
		CheckURL: cfg.Egress.CheckURL, CheckInterval: cfg.Egress.CheckInterval,
		ProxyHosts: cfg.Egress.ProxyHosts,
	}
	egressHTTP, err := egressCfg.HTTPClient(0)
	if err != nil {
		log.Error("egress_config_invalid", "error", err)
		exit(1)
	}
	egressRedis, err := cache.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		log.Warn("egress_status_store_unavailable", "error", err)
	}
	egressChecker := egress.NewChecker(egressCfg, egressHTTP, egress.NewStore(egressRedis, cfg.App.Env))
	if cfg.Queue.WorkerPool != queue.PoolGeneral && len(egressCfg.Normalize().ExpectedIPs) == 0 {
		log.Warn("egress_expected_ips_empty", "detail", "EGRESS_EXPECTED_IPS is empty: the IP check records the IP but cannot alarm")
	}
	entegratorSvc := entegrator.New(pool, vault, store, entegrator.Config{HTTP: egressHTTP}, log)
	// Zamanlanmış Easytrade senkronu (#385): kasa yoksa tarama boş döner.
	easytradeSvc := easytrade.NewService(pool, vault, cfg.Easytrade.BaseURL, nil)

	// Yükleme sonrası hesap kodu önerileri (#319): UBL işi bitince ayrı bir
	// kuyruk işi; kurala uyan kalemler inceleme kuyruğuna düşmeden uygulanır.
	reviewSvc := reviewusecase.New(reviewrepository.New(pool, queries)).
		Register(accountrulesusecase.Kind, accountrulesrepository.Applier{})
	autoSuggest := accountrulesmodule.RunAutoSuggest(pool,
		accountrulesusecase.New(accountrulesrepository.New(pool, queries), reviewSvc), log)
	ublUpload := invoiceupload.New(pool, store, nil, log).
		WithAfterProcess(accountrulesmodule.AfterUpload(queueClient, autoSuggest, log))

	worker := queue.NewWorker(cfg, log, notifSvc.Deliver).
		WithExport(exportSvc.ProcessExport).
		WithImport(importSvc.ProcessImport).
		WithBulk(bulkSvc.ProcessBulk).
		WithUBLUpload(ublUpload.Process).
		WithAccountSuggest(autoSuggest).
		// The two sweeps scan every office: deliberate row level security
		// bypass (issue #166).
		WithDocTaskReminders(systemCount(pool, "doctasks: reminders across offices", doctasksmodule.New(pool, store, notifSvc,
			doctasksmodule.Config{FrontendURL: cfg.Auth.FrontendURL}, log).SendRemindersCount)).
		WithBlockerAlerts(systemCount(pool, "officedash: blocker alerts across offices", (&officedash.Alerts{
			Q: database.NewQueries(pool), Notifier: notifSvc, FrontendURL: cfg.Auth.FrontendURL, Log: log,
		}).SendBlockerAlerts)).
		WithAgentsOfflineSweep(systemCount(pool, "agents: offline sweep across offices",
			agentsusecase.New(database.NewQueries(pool), notifSvc, cfg.Auth.FrontendURL, log).SweepOfflineCount)).
		// Bağlantı sağlığı uyarıları (#669 K): bütün ofisler (gerekçeli satır
		// güvenliği atlaması).
		WithConnectionWarnings(systemCount(pool, "warnings: connection health across offices",
			connectionWarnings(database.NewQueries(pool)).Sweep)).
		WithEntegratorSync(func(ctx context.Context, p queue.EntegratorSyncPayload) error {
			return entegratorSvc.RunTask(ctx, p.TaxpayerID, p.ConnectionUUID, p.Kind)
		}).
		WithEasytrade(
			// The sweep scans every office: deliberate row level security bypass.
			systemCount(pool, "easytrade: due connections across offices", func(ctx context.Context) (int, error) {
				return easytradeSvc.EnqueueDue(ctx, func(_ context.Context, taxpayerID int64, id uuid.UUID) error {
					return queue.EnqueueEasytradeSync(queueClient, queue.EasytradeSyncPayload{TaxpayerID: taxpayerID, ConnectionUUID: id})
				})
			}),
			func(ctx context.Context, p queue.EasytradeSyncPayload) error {
				return easytradeSvc.RunTask(ctx, p.TaxpayerID, p.ConnectionUUID)
			}).
		// Mükellef raporu e-postaları (#386): kuyruğa alınan gönderimler,
		// bütün ofisler (gerekçeli satır güvenliği atlaması).
		// Önce günü gelen zamanlanmış raporlar kuyruğa yazılır, sonra
		// kuyruk gönderilir.
		// Mutabakat mektupları (#389) aynı dakikalık taramada gönderilir.
		WithVATWatch(vatSweeps(vatWatch(pool, notifSvc, outboxStore, cfg.Auth.FrontendURL, log),
			vatReportRules(pool, notifSvc, outboxStore, cfg.Auth.FrontendURL, log))).
		WithReportDeliveries(systemCount(pool, "reports and reconciliation letters: deliveries across offices",
			mailSweeps(reportSweep(database.NewQueries(pool), mailer, cfg.Auth.FrontendURL, log),
				(&reconciliationmodule.LetterSender{Q: database.NewQueries(pool), Mail: mailer, Log: log,
					Config: reconciliationmodule.LetterConfig{Secret: cfg.Encryption.Key, FrontendURL: cfg.Auth.FrontendURL}}).SendDue))).
		WithEgressCheck(egressChecker.Run).
		WithLogPurge(logsSvc.ApplyDueRules).
		WithMobilePush(notifSvc.SendMobilePush, notifSvc.ProcessPushReceipts).
		WithSearch(
			searchIndexer.ProcessUpsert,
			searchIndexer.ProcessDelete,
			searchIndexer.ProcessReindex,
		)

	if searchIndexer.Enabled() {
		go searchIndexer.Bootstrap(ctx)
	}

	// The fixed-egress pool runs no schedulers: periodic tasks are scheduled
	// once, by the general (or all) pool, and routed by queue.
	if cfg.Queue.WorkerPool != queue.PoolFixedEgress {
		scheduler, err := queue.StartLogPurgeScheduler(cfg, log)
		if err != nil {
			log.Error("log_purge_scheduler_failed", "error", err)
			exit(1)
		}
		if err := queue.RegisterPushReceiptsSchedule(scheduler); err != nil {
			log.Error("push_receipts_scheduler_failed", "error", err)
			exit(1)
		}
		if err := queue.RegisterDocTaskRemindersSchedule(scheduler); err != nil {
			log.Error("doc_task_reminders_scheduler_failed", "error", err)
			exit(1)
		}
		if err := queue.RegisterBlockerAlertsSchedule(scheduler); err != nil {
			log.Error("blocker_alerts_scheduler_failed", "error", err)
			exit(1)
		}
		if err := queue.RegisterAgentsOfflineSweepSchedule(scheduler); err != nil {
			log.Error("agents_offline_sweep_scheduler_failed", "error", err)
			exit(1)
		}
		if err := queue.RegisterConnectionWarningsSchedule(scheduler); err != nil {
			log.Error("connection_warnings_scheduler_failed", "error", err)
			exit(1)
		}
		if err := queue.RegisterEasytradeSweepSchedule(scheduler); err != nil {
			log.Error("easytrade_sweep_scheduler_failed", "error", err)
			exit(1)
		}
		if err := queue.RegisterVATWatchSchedule(scheduler); err != nil {
			log.Error("vat_watch_scheduler_failed", "error", err)
			exit(1)
		}
		if err := queue.RegisterReportDeliveriesSchedule(scheduler); err != nil {
			log.Error("report_deliveries_scheduler_failed", "error", err)
			exit(1)
		}
		if err := queue.RegisterEgressCheckSchedule(scheduler, egressCfg.Normalize().CheckInterval); err != nil {
			log.Error("egress_check_scheduler_failed", "error", err)
			exit(1)
		}
		go func() {
			if err := scheduler.Run(); err != nil {
				log.Error("log_purge_scheduler_stopped", "error", err)
			}
		}()
	}

	// Prometheus metrics with queue depth (#179).
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer func() { _ = inspector.Close() }()
	metrics.Default.MustRegister(metrics.NewQueueCollector(inspector, queue.AllQueues()))
	if err := metrics.Default.Serve(ctx, cfg.Metrics.WorkerAddr, cfg.Metrics.Token, log); err != nil {
		log.Warn("metrics_listen_failed", "addr", cfg.Metrics.WorkerAddr, "error", err)
	}

	log.Info("worker_started",
		"app", cfg.App.Name,
		"env", cfg.App.Env,
		"concurrency", cfg.Queue.Concurrency,
		"pool", cfg.Queue.WorkerPool,
	)

	if n, err := notifSvc.ReclaimStuck(ctx, notifusecase.DefaultStuckProcessingMinutes); err != nil {
		log.Error("notification_reclaim_failed", "error", err)
	} else if n > 0 {
		log.Info("notification_reclaim_completed", "count", n)
	}
	// Queued rows no task ever claimed, incl. those lost before #309.
	if n, err := notifSvc.ReclaimQueued(ctx, queueClient, notifusecase.DefaultOrphanedQueuedMinutes); err != nil {
		log.Error("notification_requeue_failed", "error", err)
	} else if n > 0 {
		log.Info("notification_requeue_completed", "count", n)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Start()
	}()

	select {
	case <-ctx.Done():
		log.Info("worker_shutdown_signal")
		worker.Shutdown()
	case err := <-errCh:
		if err != nil {
			log.Error("worker_failed", "error", err)
			exit(1)
		}
	}
	log.Info("worker_stopped")
}

// systemCount runs a cross-office sweep in one rls.System transaction; the
// sweep's queries (database.NewQueries) join it.
// reportSweep queues the due scheduled reports, then sends the queue (#386).
func reportSweep(q *db.Queries, m mail.Sender, frontendURL string, log *slog.Logger) func(context.Context) (int, error) {
	sched := &reportsmodule.Scheduler{Q: q, Log: log}
	send := &reportsmodule.Sender{Q: q, Mail: m, Log: log, FrontendURL: frontendURL}
	return func(ctx context.Context) (int, error) {
		if n, err := sched.RunDue(ctx); err != nil {
			return 0, err
		} else if n > 0 {
			log.Info("report_schedules_queued", "queued", n)
		}
		return send.SendDue(ctx)
	}
}

// mailSweeps runs the email sweeps one after the other and adds up what
// they sent.
func mailSweeps(fns ...func(context.Context) (int, error)) func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		total := 0
		for _, fn := range fns {
			n, err := fn(ctx)
			total += n
			if err != nil {
				return total, err
			}
		}
		return total, nil
	}
}

// vatWatch builds the daily VAT watch (#780 B). Each engagement runs in its
// own rls.System transaction; the queries join it through ctx.
func vatWatch(pool *pgxpool.Pool, n vatwatch.Notifier, ob vatwatch.Outbox, frontendURL string, log *slog.Logger) *vatwatch.Watcher {
	q := database.NewQueries(pool)
	return &vatwatch.Watcher{
		Q: q, VAT: vatService(q), Config: warningsrepository.NewConfig(q), Outbox: ob, Notifier: n,
		FrontendURL: frontendURL, Log: log, Tx: systemTx(pool, "vatforecast: daily VAT watch across offices"),
	}
}

// vatService builds the VAT forecast usecase over q.
func vatService(q *db.Queries) *vatforecastusecase.Service {
	cfg := warningsrepository.NewConfig(q)
	return vatforecastusecase.New(vatforecastrepository.New(q), cfg, warningsengine.New(warningsrepository.NewWarnings(q), cfg))
}

// systemTx runs fn in one rls.System transaction.
func systemTx(pool *pgxpool.Pool, reason string) vatwatch.TxFunc {
	return func(ctx context.Context, fn func(ctx context.Context) error) error {
		return rls.System(ctx, pool, reason, func(ctx context.Context, _ pgx.Tx) error {
			return fn(ctx)
		})
	}
}

// vatReportRules builds the daily VAT report rules run (#780 B).
func vatReportRules(pool *pgxpool.Pool, n vatwatch.Notifier, ob vatwatch.Outbox, frontendURL string, log *slog.Logger) *vatreportrules.Service {
	q := database.NewQueries(pool)
	vat := vatService(q)
	return &vatreportrules.Service{
		Q: q, VAT: vat, Notices: vatnotice.New(q, vat, n, frontendURL), Outbox: ob, Notifier: n,
		Tx: systemTx(pool, "vatforecast: daily VAT report rules across offices"), FrontendURL: frontendURL, Log: log,
	}
}

// vatSweeps runs the VAT watch, then today's VAT report rules, and adds up
// the events they raised.
func vatSweeps(w *vatwatch.Watcher, rs *vatreportrules.Service) func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		n, err := w.Run(ctx)
		if err != nil {
			return n, err
		}
		m, err := rs.Run(ctx)
		return n + m, err
	}
}

// connectionWarnings builds the connection health checker over q.
func connectionWarnings(q *db.Queries) *warningsconnections.Checker {
	return warningsconnections.New(q, warningsengine.New(warningsrepository.NewWarnings(q), warningsrepository.NewConfig(q)))
}

func systemCount(pool *pgxpool.Pool, reason string, fn func(context.Context) (int, error)) func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		var n int
		err := rls.System(ctx, pool, reason, func(ctx context.Context, _ pgx.Tx) error {
			var err error
			n, err = fn(ctx)
			return err
		})
		return n, err
	}
}
