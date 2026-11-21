package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/cache"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/metrics"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	accesshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/access/handler"
	accountplanhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/planexport"
	accountplanrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/repository"
	accountplanusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	accountrulesmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules"
	accountruleshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/handler"
	accountrulesrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/repository"
	accountrulesusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/usecase"
	activityhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/activity/handler"
	activityusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/activity/usecase"
	agentshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agents/handler"
	agentsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agents/usecase"
	aimodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai"
	aihandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/handler"
	airepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/repository"
	archivemodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/archive"
	authmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth"
	authhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/identity"
	authrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/repository"
	authusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/usecase"
	authsettingsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/authsettings"
	authsettingshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/authsettings/handler"
	authsettingsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/authsettings/usecase"
	automationmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/automation"
	bankingmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	bankmatching "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/matching"
	bankposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	bankstatement "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/statement"
	bankws "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice/vakifbank"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/billing"
	bookkeepingmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/bookkeeping"
	bulkmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/bulk"
	bulkhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/bulk/handler"
	bulkusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/bulk/usecase"
	communicationmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
	copilotmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/copilot"
	doctasksmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks"
	engagementsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/engagements"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/entegrator"
	excelcustomers "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/customers"
	excelimporthandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/handler"
	excelmapping "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	excelimportusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/usecase"
	exportmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/exports"
	exporthandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/exports/handler"
	exportusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/exports/usecase"
	importmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/imports"
	importhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/imports/handler"
	importusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/imports/usecase"
	githubmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/integrations/github"
	githubhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/integrations/github/handler"
	githubusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/integrations/github/usecase"
	oauthprovidermodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/integrations/oauthprovider"
	oauthproviderhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/integrations/oauthprovider/handler"
	oauthproviderusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/integrations/oauthprovider/usecase"
	invoiceoffice "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/office"
	invoiceposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/posting"
	invoiceupload "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/upload"
	journalsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	legalhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/legal/handler"
	legalusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/legal/usecase"
	logshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/logs/handler"
	logsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/logs/usecase"
	notifmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications"
	notifhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/providers"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/unsubscribe"
	notifusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/usecase"
	officedashmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/officedash"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/officesetup"
	organizationsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations"
	orghandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/handler"
	orgrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/repository"
	orgusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/usecase"
	payrollmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/payroll"
	periodshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/handler"
	periodsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/repository"
	periodsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/usecase"
	petitionsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/petitions"
	privacymodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/privacy"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/qrlogin"
	receiptsextract "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receiptshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/handler"
	receiptsreviewlink "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/reviewlink"
	receiptsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	reconciliationmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reconciliation"
	relatedmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/related"
	reportsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reports"
	reviewhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/handler"
	reviewrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	searchhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/search/handler"
	searchusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/search/usecase"
	settingsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/settings"
	settingshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/settings/handler"
	settingsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/settings/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources/easytrade"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources/kolaybi"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources/parasut"
	storagemodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/storage"
	storagehandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/storage/handler"
	storageusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/storage/usecase"
	summarieshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/summaries/handler"
	summariesusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/summaries/usecase"
	supportmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/support"
	taxpayerportal "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/taxpayerportal"
	taxpayersmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/taxpayers"
	taxreturnsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/taxreturns"
	tenantmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/tenant"
	vatforecasthandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/handler"
	vatforecastrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/repository"
	vatforecastusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	warningsconnections "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/connections"
	warningscontrols "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/controls"
	warningsengine "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/handler"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	warningsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/usecase"
	whatsappmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/whatsapp"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/appleauth"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/bulkengine"
	bulkadapters "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/bulkengine/adapters"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/clientip"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/crypto"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/egress"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
	ioadapters "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine/adapters"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/oidc"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx/orgcache"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/outbox"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/searchengine"
	searchadapters "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/searchengine/adapters"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/stepup"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/queue"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/realtime"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/tracing"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

// Deps holds infrastructure clients wired into the HTTP server.
type Deps struct {
	DB       *pgxpool.Pool
	Queries  *db.Queries
	Redis    *redis.Client
	Queue    *queue.Client
	Storage  storage.Driver
	Realtime realtime.Publisher
	Worker   *queue.Worker
	Events   events.Bus
	// BankEndpoints overrides a bank web service's endpoint by bank id
	// (tests: internal/testfakes, #318). Empty in production: the adapter's
	// own test / production endpoint is used.
	BankEndpoints map[string]string
}

// Server is the HTTP composition root for infrastructure routes.
type Server struct {
	cfg           config.Config
	log           *slog.Logger
	db            *pgxpool.Pool
	queries       *db.Queries
	redis         *redis.Client
	queueClient   *queue.Client
	storage       storage.Driver
	realtime      realtime.Publisher
	worker        *queue.Worker
	events        events.Bus
	outboxPub     *outbox.Publisher
	outboxStop    func()
	searchClient  *searchengine.Client
	searchIndexer *searchengine.Indexer
	githubSvc     *githubusecase.Service
	oauthProvSvc  *oauthproviderusecase.Service
	authSettings  *authsettingsusecase.Service
	http          *http.Server
}

// New wires router and middleware for the API skeleton.
func New(cfg config.Config, log *slog.Logger, deps Deps) (*Server, error) {
	ipResolver, err := clientip.NewResolver(cfg.HTTP.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("httpserver: TRUSTED_PROXIES: %w", err)
	}
	mux := http.NewServeMux()
	eventBus := deps.Events
	if eventBus == nil {
		eventBus = events.NewBus(log)
	}

	s := &Server{
		cfg:         cfg,
		log:         log,
		db:          deps.DB,
		queries:     deps.Queries,
		redis:       deps.Redis,
		queueClient: deps.Queue,
		storage:     deps.Storage,
		realtime:    deps.Realtime,
		worker:      deps.Worker,
		events:      eventBus,
	}

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /v1/app/config", s.handleAppConfig)
	s.mountDocs(mux)

	tokens, err := jwt.NewManager(cfg.JWT.AccessSecret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	if err != nil {
		return nil, fmt.Errorf("httpserver: jwt: %w", err)
	}

	mailer := mail.NewSMTPSender(cfg.SMTP)
	// İletişim merkezi (#780 A): ofisin NetGSM hesabı, SMS / e-posta kaydı.
	commSvc := communicationmodule.NewFromConfig(deps.DB, cfg.Encryption.SecretsKeyring, cfg.Encryption.SecretsActiveKeyID, cfg.Encryption.Key, mailer, log)
	provs := []providers.Provider{
		providers.InappProvider{},
		providers.NewEmailProvider(mailer, cfg.Auth.FrontendURL, cfg.Encryption.Key),
		providers.RealtimeProvider{Pub: deps.Realtime},
		communicationmodule.SMSProvider{Svc: commSvc},
		providers.NoopProvider{Name: "push", Log: log},
	}
	secretBox, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		return nil, fmt.Errorf("httpserver: encryption: %w", err)
	}
	notifSvc := notifusecase.New(deps.Queries, deps.Queue, provs, log)
	notifSvc.WithActionSigner(cfg.JWT.AccessSecret, 0)
	notifSvc.WithUnsubscribe(unsubscribe.New(cfg.Encryption.Key, cfg.Auth.FrontendURL))
	notifSvc.WithSecretBox(secretBox)
	notifSvc.WithVAPID(notifusecase.VAPIDConfig{
		PublicKey:  cfg.VAPID.PublicKey,
		PrivateKey: cfg.VAPID.PrivateKey,
		Subject:    cfg.VAPID.Subject,
	})
	notifSvc.WithExpo(notifusecase.ExpoConfig{AccessToken: cfg.Expo.AccessToken})

	repo := authrepo.NewPostgres(deps.DB, deps.Queries)
	uc := authusecase.New(repo, tokens)
	uc.SetNotifier(notifSvc)
	uc.SetLogger(log)

	searchReg := searchengine.NewRegistry(
		searchadapters.NewUsers(deps.Queries),
		searchadapters.NewRoles(deps.Queries),
	)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, deps.Queue, log)
	s.searchClient = searchClient
	s.searchIndexer = searchIndexer
	uc.SetSearchIndexer(searchIndexer)

	rtIssuer, err := realtime.NewTokenIssuer(cfg.Centrifugo)
	if err != nil {
		return nil, fmt.Errorf("httpserver: realtime tokens: %w", err)
	}
	uc.SetRealtimeHints(authusecase.RealtimeHints{
		Enabled: rtIssuer.Enabled(),
		WSURL:   cfg.Centrifugo.WSURL,
	})

	activityRec := activity.NewRecorder(deps.Queries, log)

	stepUpStore := stepup.NewStore(deps.Redis, cfg.App.Env)
	var stepUpWebAuthn *stepup.WebAuthn
	stepUpOrigins := cfg.CORS.AllowedOrigins
	if len(stepUpOrigins) == 0 {
		stepUpOrigins = []string{strings.TrimRight(cfg.Auth.FrontendURL, "/")}
	}
	if webauthn, err := stepup.NewWebAuthn(stepup.WebAuthnConfig{
		RPID:          cfg.Auth.WebAuthnRPID,
		RPDisplayName: cfg.App.Name,
		RPOrigins:     stepUpOrigins,
	}); err == nil {
		stepUpWebAuthn = webauthn
	} else {
		log.Warn("stepup_webauthn_disabled", "error", err)
	}
	stepUpSvc := stepup.NewService(deps.Queries, stepUpStore, stepUpWebAuthn, repo)
	stepUpSvc.SetSecretBox(secretBox)

	githubSvc := githubusecase.New(deps.Queries, secretBox)
	oauthProvSvc := oauthproviderusecase.New(deps.Queries, secretBox)
	authSettingsSvc := authsettingsusecase.New(deps.Queries)
	uc.SetAuthSettings(authSettingsSvc)
	uc.SetAccessPolicy(stepUpSvc)
	uc.SetSecretBox(secretBox, cfg.App.Name)
	oauthUC := authusecase.NewOAuth(repo, secretBox)
	uc.SetReviewAccounts(cfg.Auth.ReviewAccounts)
	// Apple signing key: admin-uploaded .p8 (DB) first, AUTH_APPLE_* env fallback.
	appleClient, err := appleauth.NewResolver(oauthProvSvc, appleauth.Config{
		TeamID: cfg.Auth.AppleTeamID, KeyID: cfg.Auth.AppleKeyID, PrivateKey: cfg.Auth.ApplePrivateKey,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("httpserver: apple sign-in key: %w", err)
	}
	oauthProvSvc.SetAppleKeys(appleClient)
	uc.SetNativeOAuth(authusecase.NativeOAuthConfig{
		Verifier:        oidc.New(oidc.DefaultProviders(), nil),
		Apple:           appleClient,
		Clients:         oauthProvSvc,
		AppleClientIDs:  cfg.Auth.AppleNativeClientIDs,
		GoogleClientIDs: cfg.Auth.GoogleNativeClientIDs,
	})
	s.githubSvc = githubSvc
	s.oauthProvSvc = oauthProvSvc
	s.authSettings = authSettingsSvc

	// One limiter for the whole API (#180): the in-process store must be
	// shared, and abuse events go to the activity log.
	limiter := newRateLimiter(cfg, deps, activityRec)
	h := authhandler.New(uc, oauthUC, githubSvc, oauthProvSvc, authSettingsSvc, cfg.Auth.AdapterSecret, stepUpSvc, activityRec)
	h.SetRateLimiter(limiter)
	loader := identity.Loader{UC: uc}
	authmodule.RegisterRoutes(mux, h, tokens, loader, stepUpSvc)

	// Organizations (tenant core). Platform management is always mounted;
	// the /v1/tenant/* shell only with TENANCY_ENABLED.
	// KVKK metinleri ve onay kayıtları (issue #57).
	legalSvc := legalusecase.NewPostgres(deps.DB)
	orgRepo := orgrepository.NewPostgres(deps.DB, deps.Queries)
	orgSvc := orgusecase.New(orgRepo, nil)
	organizationsmodule.RegisterRoutes(mux, orghandler.New(orgSvc, activityRec), tokens, loader)
	var (
		tenantResolver     orgctx.Resolver
		tenantHandler      *tenantmodule.Handler
		engagementResolver engagementctx.Resolver
		engagementHandler  *engagementsmodule.Handler
		taxpayerResolver   taxpayerctx.Resolver
		memberHandler      *taxpayersmodule.MemberHandler
		onboardingHandler  *taxpayersmodule.OnboardingHandler
		importHandler      *taxpayersmodule.ImportHandler
		historyHandler     *taxpayersmodule.HistoryHandler
	)
	if cfg.Tenancy.Enabled {
		// Invitation emails go through the notifications queue (never SMTP
		// inline); links point at the frontend's /invite/{token} page.
		orgSvc.SetInvitations(orgRepo, notifSvc, orgusecase.InvitationConfig{
			TTL: cfg.Tenancy.InviteTTL, FrontendURL: cfg.Auth.FrontendURL,
		}, log)
		tenantHandler = tenantmodule.NewHandler(orgSvc, activityRec, limiter)
		// /me lists memberships only when the /t shell exists: the frontend
		// routes members into it, which would dead-end with tenancy off.
		uc.SetOrganizationsLister(orgSvc)
		// Mükellef users (issue #4): office invitations from an engagement,
		// /me taxpayers[] and the X-Taxpayer shell. An invitation sign-up
		// accepts both office and mükellef invitation tokens.
		members := taxpayersmodule.NewMembers(taxpayersmodule.NewMemberPostgres(deps.DB), notifSvc,
			taxpayersmodule.InvitationConfig{TTL: cfg.Tenancy.InviteTTL, FrontendURL: cfg.Auth.FrontendURL}, log)
		uc.SetTaxpayersLister(members)
		taxpayerResolver = members
		memberHandler = taxpayersmodule.NewMemberHandler(members, limiter)
		orgCache := orgcache.New(orgSvc, deps.Redis, cfg.Tenancy.CacheTTL, cfg.App.Env)
		orgSvc.SetInvalidator(orgCache)
		// Add-mükellef flow (issue #7): consent tokens also open sign-up.
		onboarding := taxpayersmodule.NewOnboarding(deps.DB, notifSvc, orgCache,
			taxpayersmodule.OnboardingConfig{TTL: cfg.Tenancy.InviteTTL, FrontendURL: cfg.Auth.FrontendURL}, log)
		uc.SetInvitationGate(taxpayersmodule.ChainGates(orgSvc, members, onboarding))
		onboardingHandler = taxpayersmodule.NewOnboardingHandler(onboarding, limiter).WithNotice(legalSvc)
		// Kurulum sihirbazı (issue #273): Excel'den toplu mükellef aktarımı.
		importHandler = taxpayersmodule.NewImportHandler(taxpayersmodule.NewImporter(deps.DB, onboarding)).WithLimiter(limiter)
		// Office change (issue #8): history shares and archive reads.
		historyHandler = taxpayersmodule.NewHistoryHandler(taxpayersmodule.NewHistory(deps.DB).WithConsents(legalSvc))
		tenantResolver = orgCache
		// Engagements (issue #2): assignment changes bump the organization's
		// cache version, so engagement and membership answers expire together.
		engSvc := engagementsmodule.New(engagementsmodule.NewPostgres(deps.Queries), orgCache)
		engagementResolver = orgcache.NewEngagementCache(engSvc, orgCache)
		engagementHandler = engagementsmodule.NewHandler(engSvc)
	}
	tenantmodule.RegisterRoutes(mux, tokens, loader, tenantResolver, tenantHandler)
	// Faturalama (issue #52): plan, aktif mükellef sayımı, taslak, salt okunur kip.
	billingSvc := billing.NewService(deps.DB)
	middleware.SetOrganizationPolicy(billingSvc)
	billing.RegisterRoutes(mux, tokens, loader, tenantResolver, billing.NewHandler(billingSvc))
	engagementsmodule.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver, engagementHandler)
	// Taxpayers (issue #3): platform catalog always; the office-side profile
	// of an engagement only with tenancy.
	taxpayerSvc := taxpayersmodule.New(taxpayersmodule.NewPostgres(deps.Queries))
	// Office-side taxpayer reads run under Postgres RLS (issue #6).
	taxpayerSvc.SetRLS(deps.DB)
	taxpayerHandler := taxpayersmodule.NewHandler(taxpayerSvc)
	taxpayersmodule.RegisterPlatformRoutes(mux, tokens, loader, taxpayerHandler)
	taxpayersmodule.RegisterTenantRoutes(mux, tokens, loader, tenantResolver, engagementResolver, taxpayerHandler)
	// UBL / ZIP fatura yükleme (issue #11), aktif engagement zinciri arkasında.
	var ublEnq invoiceupload.Enqueuer
	if deps.Queue != nil {
		ublEnq = deps.Queue
	}
	// Kilitli döneme gelen fatura değiştirilmez, değişiklik talebi olur (#32).
	ublSvc := invoiceupload.New(deps.DB, deps.Storage, ublEnq, log).WithPeriodGuard(periodsrepository.NewGuard())
	invoiceupload.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver, invoiceupload.NewHandler(ublSvc))
	// Ofis paneli fatura listesi ve detayı (#142).
	var ublDownload invoiceoffice.Downloader
	if deps.Storage != nil {
		ublDownload = func(r *http.Request, path string) (io.ReadCloser, int64, error) {
			return deps.Storage.Download(r.Context(), path)
		}
	}
	invoiceoffice.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver, invoiceoffice.NewHandler(deps.Queries, ublDownload).WithActivity(activityRec).WithNotifier(notifSvc, cfg.Auth.FrontendURL))
	// Satır güvenliği zorlanmış tabloların uçları istek başına tek kapsamlı
	// işlemde çalışır (issue #166, rls.Middleware).
	var rlsScope func(http.Handler) http.Handler
	if deps.DB != nil {
		rlsScope = rls.Middleware(deps.DB, database.VisibleTaxpayers(deps.DB), log)
		if cfg.DB.RLSRequestScope {
			// Every tenant route: RequireOrganization opens the scope,
			// RequireEngagement narrows it (issue #166).
			middleware.SetTenantScope(rlsScope)
			middleware.SetTaxpayerScope(rlsScope)
		}
	}
	// Yevmiye taslağı ve Luca / Zirve / Excel çıktısı (#143, #26). Hedef hesap
	// planı (#30) yüklüyse çıktıda ACCOUNT_NOT_IN_TARGET_PLAN denetlenir.
	if deps.Storage != nil {
		// #319: the gate normalizes codes, rejects group accounts and records
		// the result as a warning.
		planCfg := warningsrepository.NewConfig(deps.Queries)
		planGate := accountplanusecase.NewGate(accountplanusecase.New(accountplanrepository.New(deps.DB, deps.Queries)),
			warningsengine.New(warningsrepository.NewWarnings(deps.Queries), planCfg), planCfg)
		journalSvc := journalsmodule.New(deps.DB, deps.Storage).WithPlanGate(planGate.ExportGate())
		journalsmodule.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver, journalsmodule.NewHandler(journalSvc, deps.Storage), rlsScope)
		// Faturadan muhasebe fişi (#669): alış / satış faturası taslak fişe.
		invoiceposting.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
			invoiceposting.NewHandler(invoiceposting.NewService(deps.Queries, journalSvc)))
	}
	// KVKK veri sahibi talepleri (#193): dışa aktarma ve anonimleştirme, yönetici onaylı.
	privacymodule.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		privacymodule.NewHandler(privacymodule.New(deps.DB, privacymodule.Config{
			AccountingRetentionYears: cfg.Privacy.AccountingRetentionYears, ExportTTL: cfg.Privacy.ExportTTL,
		}), activityRec))
	if s.worker != nil {
		s.worker.WithUBLUpload(ublSvc.Process)
	}
	// Banka ekstresi yükleme (issue #19): MT940 / Excel / CSV / metin PDF.
	bankSvc := bankingmodule.NewService(deps.DB, nil)
	bankImporter := bankstatement.NewImporter(deps.DB, bankSvc)
	bankstatement.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		bankstatement.NewHandler(bankImporter, bankSvc, deps.Queries))
	// Paraşüt adaptörü (issue #47): OAuth ile mükellef yetkisi, salt okuma.
	var sourceVault secrets.Vault
	if kr, err := secrets.ParseKeyring(cfg.Encryption.SecretsKeyring, cfg.Encryption.SecretsActiveKeyID, cfg.Encryption.Key); err != nil {
		log.Warn("source connections: credential vault disabled", "error", err)
	} else {
		// Secrets are platform records (no tenant scope policy): the vault
		// never joins a request's row level security transaction.
		sourceVault = secrets.New(kr, secrets.NewPGStore(db.New(deps.DB)))
	}
	parasut.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		parasut.NewHandler(parasut.NewService(deps.DB, sourceVault, parasut.App{
			BaseURL: cfg.Parasut.BaseURL, ClientID: cfg.Parasut.ClientID,
			ClientSecret: cfg.Parasut.ClientSecret, RedirectURL: cfg.Parasut.RedirectURL,
		}, nil)))
	// Easytrade adaptörü (issue #46).
	easytradeSvc := easytrade.NewService(deps.DB, sourceVault, cfg.Easytrade.BaseURL, nil)
	easytrade.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver, easytrade.NewHandler(easytradeSvc))
	// İmzalı Easytrade webhook'u (#385): EASYTRADE_WEBHOOK_SECRET boşsa kapalı (404).
	var easytradeEnqueue func(context.Context, int64, uuid.UUID) error
	if deps.Queue != nil {
		easytradeEnqueue = func(_ context.Context, taxpayerID int64, id uuid.UUID) error {
			return queue.EnqueueEasytradeSync(deps.Queue, queue.EasytradeSyncPayload{TaxpayerID: taxpayerID, ConnectionUUID: id})
		}
	}
	easytrade.RegisterWebhook(mux, easytrade.NewWebhook(easytradeSvc, cfg.Easytrade.WebhookSecret, easytradeEnqueue))
	// e-Fatura entegratör bağlantısı (#518): kimlik kasada, ilk çekim
	// sabit IP'li işçide.
	var entegratorEnqueue entegrator.Enqueuer
	if deps.Queue != nil {
		entegratorEnqueue = func(_ context.Context, taxpayerID int64, id uuid.UUID, kind string) error {
			return queue.EnqueueEntegratorSync(deps.Queue, queue.EntegratorSyncPayload{TaxpayerID: taxpayerID, ConnectionUUID: id, Kind: kind})
		}
	}
	entegrator.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		entegrator.NewHandler(entegrator.NewConnections(deps.DB, sourceVault, entegratorEnqueue)))
	// KolayBi adaptörü (issue #48).
	kolaybi.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		kolaybi.NewHandler(kolaybi.NewService(deps.DB, sourceVault, kolaybi.App{BaseURL: cfg.KolayBi.BaseURL, Channel: cfg.KolayBi.Channel}, nil)))
	// Banka web servisi (issue #49): şube talimatı + sabit IP ile ilk banka.
	// Kimlik bilgileri sır kasasında (purpose bank_connection). Banka
	// çağrıları egress vekilinden (EGRESS_PROXY_URL, EGRESS_PROXY_HOSTS)
	// sabit IP ile çıkar (issue #256); diğer hostlar doğrudan.
	bankHTTP, err := egress.Config{
		ProxyURL: cfg.Egress.ProxyURL, CheckURL: cfg.Egress.CheckURL, ProxyHosts: cfg.Egress.ProxyHosts,
	}.HTTPClient(2 * time.Minute)
	if err != nil {
		return nil, err
	}
	bankws.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		bankws.NewHandler(bankws.NewService(deps.DB, sourceVault, bankSvc, bankws.Registry{
			"vakifbank": func(env string) bankws.Adapter {
				if ep := deps.BankEndpoints["vakifbank"]; ep != "" {
					return vakifbank.New(ep, bankHTTP)
				}
				if env == bankws.EnvTest {
					return vakifbank.New(vakifbank.TestEndpoint, bankHTTP)
				}
				return vakifbank.New(vakifbank.ProductionEndpoint, bankHTTP)
			},
		}, bankws.Options{EgressIPs: cfg.Bank.EgressIPs, TechContact: cfg.Bank.TechContact})))
	taxpayersmodule.RegisterMemberRoutes(mux, tokens, loader, tenantResolver, engagementResolver, taxpayerResolver, memberHandler)
	taxpayersmodule.RegisterOnboardingRoutes(mux, tokens, loader, tenantResolver, taxpayerResolver, onboardingHandler)
	taxpayersmodule.RegisterImportRoutes(mux, tokens, loader, tenantResolver, importHandler)
	taxpayersmodule.RegisterHistoryRoutes(mux, tokens, loader, tenantResolver, engagementResolver, taxpayerResolver, historyHandler)
	// Mükellef web paneli (issue #39): faturalar, fişler, belge yükleme.
	portalH := taxpayerportal.NewHandler(deps.Queries, ublSvc)
	taxpayerportal.RegisterRoutes(mux, tokens, loader, taxpayerResolver, portalH)
	// Eksik belge görevleri (issue #36): tespit, ofis görevi, mükellef yüklemesi.
	docTasksSvc := doctasksmodule.New(deps.DB, deps.Storage, notifSvc,
		doctasksmodule.Config{FrontendURL: cfg.Auth.FrontendURL}, log)
	if s.worker != nil {
		s.worker.WithDocTaskReminders(docTasksSvc.SendRemindersCount)
	}
	realtime.RegisterRoutes(mux, realtime.NewHandler(rtIssuer, uc), tokens, loader)
	// Dilekçe üretimi (issue #56): şablon + engagement alanları, önizleme ve PDF.
	petitionsmodule.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		petitionsmodule.NewHandler(petitionsmodule.New(deps.DB)))
	// Fixed-egress status (issue #45): the last public IP check of the
	// egress worker pool.
	mux.Handle("GET /v1/platform/egress", middleware.Chain(
		egress.StatusHandler(egress.Config{
			ProxyURL: cfg.Egress.ProxyURL, ExpectedIPs: cfg.Egress.ExpectedIPs,
			CheckURL: cfg.Egress.CheckURL, CheckInterval: cfg.Egress.CheckInterval,
		}, egress.NewStore(deps.Redis, cfg.App.Env)),
		middleware.Authenticate(tokens, loader),
		middleware.RequirePermission(rbac.PermPlatformSettingsRead),
	))

	qrGeo, err := qrlogin.NewLocator(cfg.Auth.QRGeoIPDB, cfg.Auth.QRTrustGeoHeaders)
	if err != nil {
		// Location is a hint only: keep QR sign-in up and show the IP.
		log.Warn("qrlogin_geoip_unavailable", "error", err)
	}
	var qrIssuer qrlogin.TokenIssuer
	if rtIssuer != nil {
		qrIssuer = rtIssuer
	}
	qrSvc := qrlogin.NewService(qrlogin.NewStore(deps.Redis, cfg.App.Env), uc, qrIssuer, deps.Realtime, cfg.Auth.FrontendURL, log)
	qrlogin.RegisterRoutes(mux, qrlogin.NewHandler(qrSvc, qrGeo, limiter), tokens, loader)

	nh := notifhandler.New(notifSvc)
	nh.SetRateLimiter(limiter)
	notifmodule.RegisterRoutes(mux, nh, tokens, loader)
	// Ofis ve mükellef paneli bildirim kutusu (#333).
	notifmodule.RegisterScopedRoutes(mux, nh, tokens, loader, tenantResolver, taxpayerResolver)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log)

	ioReg := ioengine.NewRegistry(
		ioadapters.NewUsers(deps.Queries),
		ioadapters.NewRoles(deps.Queries),
		ioadapters.NewNotifications(deps.Queries),
		ioadapters.NewActivity(deps.Queries),
	)
	exportSvc := exportusecase.New(deps.Queries, deps.Storage, ioReg, deps.Queue, notifSvc, activityRec, log)
	importSvc := importusecase.New(deps.Queries, deps.Storage, ioReg, deps.Queue, notifSvc, activityRec, log)
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(deps.Queries),
		bulkadapters.NewRoles(deps.Queries),
	)
	bulkSvc := bulkusecase.New(deps.Queries, bulkReg, deps.Queue, notifSvc, activityRec, cfg.Bulk, log)
	logsSvc := logsusecase.New(deps.Queries)
	if s.worker != nil {
		s.worker.WithExport(exportSvc.ProcessExport).
			WithImport(importSvc.ProcessImport).
			WithBulk(bulkSvc.ProcessBulk).
			WithLogPurge(logsSvc.ApplyDueRules).
			WithMobilePush(notifSvc.SendMobilePush, notifSvc.ProcessPushReceipts)
		if searchIndexer != nil {
			s.worker.WithSearch(
				searchIndexer.ProcessUpsert,
				searchIndexer.ProcessDelete,
				searchIndexer.ProcessReindex,
			)
		}
	}
	exportmodule.RegisterRoutes(mux, exporthandler.New(exportSvc), tokens, loader, stepUpSvc)
	importmodule.RegisterRoutes(mux, importhandler.New(importSvc), tokens, loader)
	bulkmodule.RegisterRoutes(mux, bulkhandler.New(bulkSvc), tokens, loader)
	settingsmodule.RegisterRoutes(mux, settingshandler.New(
		settingsusecase.New(deps.Queries),
		deps.Storage,
		settingsusecase.NewSystemService(cfg, deps.DB, deps.Redis, deps.Storage, searchClient),
	), tokens, loader)
	authsettingsmodule.RegisterRoutes(mux, authsettingshandler.New(authSettingsSvc, activityRec), tokens, loader)
	githubmodule.RegisterRoutes(mux, githubhandler.New(githubSvc, activityRec), tokens, loader)
	oauthprovidermodule.RegisterRoutes(mux, oauthproviderhandler.New(oauthProvSvc, activityRec), tokens, loader)
	// Self-registering modules (docs/design/module-registry.md). Modules move
	// here one at a time from the explicit RegisterRoutes calls above.
	modRouter := module.NewRouter(module.Deps{Mux: mux, Tokens: tokens, Loader: loader, Resolver: tenantResolver})
	searchSvc := searchusecase.New(searchClient, searchReg, log)
	// Appliers are registered by the modules that own suggestion targets.
	aiSvc := aimodule.NewService(cfg.AI, airepository.NewPostgres(deps.Queries).WithLedgerPool(deps.DB), log)
	// Banka ekstresi şablon analizi (#383): bilinmeyen biçimde AI kolon eşleme.
	bankImporter.WithColumnSuggester(bankstatement.NewAIColumnMapper(aiSvc))
	// Appliers are registered by the modules that own suggestion targets.
	reviewSvc := reviewusecase.New(reviewrepository.New(deps.DB, deps.Queries)).
		Register(receiptsreviewlink.Kind, receiptsreviewlink.Applier{}).
		Register(bankmatching.SuggestionKind, bankmatching.Applier{}).
		Register(bankposting.SuggestionKind, bankposting.Applier{}).
		Register(accountrulesusecase.Kind, accountrulesrepository.Applier{}).
		Register(reconciliationmodule.SuggestionKind, reconciliationmodule.Applier{})
	warningEngine := warningsengine.New(warningsrepository.NewWarnings(deps.Queries), warningsrepository.NewConfig(deps.Queries))
	// Hesap kodu önerileri (#25); yükleme sonrası kendiliğinden (#319).
	accountRulesSvc := accountrulesusecase.New(accountrulesrepository.New(deps.DB, deps.Queries), reviewSvc)
	autoSuggest := accountrulesmodule.RunAutoSuggest(deps.DB, accountRulesSvc, log)
	ublSvc.WithAfterProcess(accountrulesmodule.AfterUpload(ublEnq, autoSuggest, log))
	if s.worker != nil {
		s.worker.WithAccountSuggest(autoSuggest)
	}
	// Banka hareketi → mizan hesabı (#383): takma ad hafızası, hesap önerisi,
	// inceleme ve onaylananlardan yevmiye taslağı (Luca / Zirve çıktısına girer).
	bankFlow := bankposting.NewWorkflow(deps.DB, reviewSvc, journalsmodule.New(deps.DB, deps.Storage))
	bankposting.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		bankposting.NewHandler(bankposting.NewService(deps.DB), bankFlow))
	// Banka–fatura eşleme ve BNK kontrolleri (#21); tek hareket ekranı
	// hareketi hesaba da işler (#565).
	bankmatching.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		bankmatching.NewHandler(bankmatching.NewService(deps.DB, reviewSvc, warningEngine,
			bankmatching.Options{FeeTolerance: decimal.RequireFromString("25")}).WithAccounts(bankFlow)).WithActivity(activityRec))
	receiptsSvc := receiptsusecase.New(deps.DB, deps.Storage, receiptsextract.New(aiSvc),
		receiptsreviewlink.NewReviewer(reviewSvc), warningEngine, log)
	if deps.Storage != nil {
		// #384: onaylanan masraf / Z raporu yevmiye taslağına dönüşür.
		receiptsSvc.WithJournals(journalsmodule.New(deps.DB, deps.Storage))
	}
	// Mükellef paneli ve mobil fiş yüklemesi aynı servisi kullanır (#199).
	portalH.WithReceipts(receiptsSvc)
	// Personel, bordro ve SGK (#382): ofis uçları ve mükellef panelinde bordro PDF'leri.
	var payrollStore payrollmodule.ObjectStore
	if deps.Storage != nil {
		payrollStore = deps.Storage
	}
	payrollSvc := payrollmodule.New(database.Ambient{Pool: deps.DB}, payrollStore, cfg.Encryption.Key).
		WithJournals(journalsmodule.New(deps.DB, deps.Storage)).
		WithAI(excelmapping.NewAIMapper(aiSvc))
	payrollH := payrollmodule.NewHandler(payrollSvc)
	payrollmodule.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver, payrollH)
	payrollmodule.RegisterTaxpayerRoutes(mux, tokens, loader, taxpayerResolver, payrollH)
	// Beyanname formatına uygunluk (#390): KDV1, MUHSGK ve Ba/Bs önizlemesi, beyan kayıtları.
	// Tahakkuk PDF'i (#575) aynı nesne deposunda durur.
	taxreturnsSvc := taxreturnsmodule.New(deps.DB).WithAccounts(taxPaymentBooker{bankFlow})
	if deps.Storage != nil {
		taxreturnsSvc.WithStore(deps.Storage)
	}
	taxreturnsmodule.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		taxreturnsmodule.NewHandler(taxreturnsSvc))
	// Defter türü ve işletme hesabı (#780).
	bookkeepingmodule.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver,
		bookkeepingmodule.NewHandler(bookkeepingmodule.New(deps.DB)))
	// Belge arşivi ve İSG evrakları (#393).
	var archiveStore archivemodule.ObjectStore
	if deps.Storage != nil {
		archiveStore = deps.Storage
	}
	archiveH := archivemodule.NewHandler(archivemodule.New(database.Ambient{Pool: deps.DB}, archiveStore))
	archivemodule.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver, archiveH)
	// Mükellef paneli: "Mükellef de görsün" işaretli belgeler (#682).
	archivemodule.RegisterTaxpayerRoutes(mux, tokens, loader, taxpayerResolver, archiveH)
	// Mutabakat (#389): mizan – kaynak çifte mutabakatı ve cari mutabakat.
	reconciliationH := reconciliationmodule.NewHandler(reconciliationmodule.New(database.Ambient{Pool: deps.DB}, reviewSvc).
		WithLetters(reconciliationmodule.LetterConfig{Secret: cfg.Encryption.Key, FrontendURL: cfg.Auth.FrontendURL}))
	reconciliationmodule.RegisterRoutes(mux, tokens, loader, tenantResolver, engagementResolver, reconciliationH)
	reconciliationmodule.RegisterPublicRoutes(mux, reconciliationH)
	if err := modRouter.Mount(selfRegisteredModules(moduleHandlers{
		access:   accesshandler.New(stepUpSvc, activityRec),
		search:   searchhandler.New(searchSvc),
		activity: activityhandler.New(activityusecase.New(deps.Queries)),
		logs:     logshandler.New(logsSvc),
		ai:       aihandler.New(aiSvc),
		// Bordro, masraf, Z raporu ve banka hareketi de ortak Excel akışından
		// içe aktarılır; satırları sahibi olan modül yazar (#395).
		excel: excelimporthandler.New(excelimportusecase.New(deps.DB, deps.Storage, aiSvc, log).
			WithTarget(excelmapping.Payroll, payrollSvc.ExcelTarget()).
			WithTarget(excelmapping.Expenses, receiptsSvc.ExpenseTarget()).
			WithTarget(excelmapping.RetailSales, receiptsSvc.RetailSaleTarget()).
			WithTarget(excelmapping.BankTransactions, bankImporter.ExcelTarget()).
			WithTarget(excelmapping.Customers, excelcustomers.New(deps.DB))),
		warnings: warningshandler.New(warningsusecase.NewRules(warningsrepository.NewConfig(deps.Queries))).
			WithWarnings(warningsusecase.NewWarnings(warningsrepository.NewWarnings(deps.Queries))).
			// Bağlantı sağlığı uyarıları (#669 K) kontrollerle birlikte tazelenir.
			WithControls(warningscontrols.New(deps.Queries, warningEngine).
				WithConnections(warningsconnections.New(deps.Queries, warningEngine))).
			WithActivity(activityRec),
		review:       reviewhandler.New(reviewSvc, activityRec),
		receipts:     receiptshandler.New(receiptsSvc),
		accountrules: accountruleshandler.New(accountRulesSvc),
		accountplan:  accountplanhandler.New(accountplanusecase.New(accountplanrepository.New(deps.DB, deps.Queries))),
		planexport:   planexport.NewHandler(planexport.New(deps.DB)),
		periods:      periodshandler.New(periodsusecase.New(periodsrepository.New(deps.DB))),
		summaries:    summarieshandler.New(summariesusecase.New(deps.Queries, aiSvc, log)),
		// Mükellef raporu (#386).
		reports: reportsmodule.NewHandler(deps.Queries, engagementResolver),
		// Co-pilot (#392): planlayıcı AI varsa onu, yoksa kural tabanlı çözümleyiciyi kullanır.
		copilot: newCopilotHandler(deps.Queries, aiSvc, log),
		// Bildirimi zamanlanmış tarama (worker) gönderir; HTTP tarafına gerekmez.
		agents:        agentshandler.New(agentsusecase.New(deps.Queries, nil, "", log)),
		legal:         legalhandler.New(legalSvc),
		doctasks:      doctasksmodule.NewHandler(docTasksSvc),
		officedash:    &officedashmodule.Handler{Q: deps.Queries},
		officeClients: &officedashmodule.ClientsHandler{Q: deps.Queries, Store: deps.Storage},
		officeReview:  &officedashmodule.ReviewHandler{Q: deps.Queries},
		setup:         officesetup.NewHandler(officesetup.New(deps.DB)),
		support:       supportmodule.NewHandler(deps.Queries, supportmodule.IdentitySecretFromEnv()),
		communication: communicationmodule.NewHandler(commSvc),
		// Otomasyon kuralları (#780 C): taslak onayı istekte gönderir.
		automation: automationmodule.NewHandler(automationmodule.New(deps.DB, commSvc, notifSvc, cfg.Auth.FrontendURL, log)),
		// WhatsApp Web uzantısı (#388): eşleştirme QR girişiyle aynı oturum modeli.
		whatsapp: whatsappmodule.NewHandler(
			whatsappmodule.New(deps.DB, receiptsSvc, docTasksSvc, deps.Storage, log),
			whatsappmodule.NewPairings(deps.Redis, cfg.App.Env, uc), limiter),
		vat: vatforecasthandler.New(vatforecastusecase.New(
			vatforecastrepository.New(deps.Queries), warningsrepository.NewConfig(deps.Queries), warningEngine,
		), activityRec),
		engagements: engagementResolver,
		related:     relatedmodule.NewHandler(relatedmodule.New(deps.Queries)),
		rlsScope:    rlsScope,
		taxpayers:   taxpayerResolver,
	})...); err != nil {
		return nil, fmt.Errorf("httpserver: %w", err)
	}
	storagemodule.RegisterRoutes(
		mux,
		storagehandler.New(storageusecase.New(deps.Storage, deps.Queries, log), cfg.Storage.MaxBytes).WithAudit(activityRec),
		tokens,
		loader,
	)

	outboxStore := outbox.NewStore(deps.DB, deps.Queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log)
	s.outboxPub = outboxPub

	s.http = &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      tracing.HTTP(middleware.RequestID(clientip.Middleware(ipResolver)(middleware.ServerErrors(log)(metrics.Default.HTTP(limiter.UploadGuard(periodq.Middleware(mux))))))),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}
	return s, nil
}

// Start listens until the server stops. Optionally starts an in-process queue worker.
func (s *Server) Start() error {
	if s.outboxPub != nil {
		s.outboxStop = s.outboxPub.StartRun(context.Background())
	}
	if s.worker != nil {
		go func() {
			if err := s.worker.Start(); err != nil {
				s.log.Error("queue_worker_failed", "error", err)
			}
		}()
	}
	if s.searchIndexer != nil {
		go s.searchIndexer.Bootstrap(context.Background())
	}
	s.log.Info("http_listen", "addr", s.cfg.HTTP.Addr, "env", s.cfg.App.Env)
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("httpserver: listen: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the HTTP server, queue worker, and queue client.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.outboxStop != nil {
		s.outboxStop()
	}
	if s.worker != nil {
		s.worker.Shutdown()
	}
	if s.queueClient != nil {
		_ = s.queueClient.Close()
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) handleAppConfig(w http.ResponseWriter, r *http.Request) {
	settingsSvc := settingsusecase.New(s.queries)
	letterhead, _ := settingsSvc.PublicLetterhead(r.Context())

	registrationEnabled := false
	passwordLogin := true
	passwordRegister := false
	passkeyLogin := true
	if s.authSettings != nil {
		if p, err := s.authSettings.Policy(r.Context()); err == nil {
			registrationEnabled = p.RegistrationEnabled
			passwordLogin = p.PasswordLoginEnabled
			passwordRegister = p.RegistrationEnabled && p.PasswordRegisterEnabled
			passkeyLogin = p.PasskeyLoginEnabled
		}
	}

	githubLogin, githubRegister := false, false
	if s.githubSvc != nil {
		githubLogin, _ = s.githubSvc.IsAuthEnabled(r.Context())
		reg, _ := s.githubSvc.IsRegisterEnabled(r.Context())
		githubRegister = registrationEnabled && reg
	}

	method := func(login, register bool) map[string]bool {
		return map[string]bool{"login": login, "register": register}
	}
	oauthMethod := func(provider string) map[string]bool {
		login, register := false, false
		if s.oauthProvSvc != nil {
			login, _ = s.oauthProvSvc.IsLoginEnabled(r.Context(), provider)
			reg, _ := s.oauthProvSvc.IsRegisterEnabled(r.Context(), provider)
			register = registrationEnabled && reg
		}
		return method(login, register)
	}

	responseJSON := map[string]any{
		"mode":                 "platform",
		"name":                 s.cfg.App.Name,
		"vapid_configured":     s.cfg.VAPID.PublicKey != "" && s.cfg.VAPID.PrivateKey != "",
		"github_auth_enabled":  githubLogin, // legacy
		"registration_enabled": registrationEnabled,
		"auth_methods": map[string]any{
			"password": method(passwordLogin, passwordRegister),
			"passkey":  map[string]bool{"login": passkeyLogin},
			"github":   method(githubLogin, githubRegister),
			"google":   oauthMethod("google"),
			"facebook": oauthMethod("facebook"),
			"apple":    oauthMethod("apple"),
		},
		"letterhead": letterhead,
		// The frontend mounts the /t/{slug} shell only when tenancy is on.
		"tenancy": map[string]bool{
			"enabled":    s.cfg.Tenancy.Enabled,
			"self_serve": s.cfg.Tenancy.Enabled && s.cfg.Tenancy.SelfServe,
		},
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    responseJSON,
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{
		"postgres": "ok",
		"redis":    "ok",
	}
	ready := true

	if err := database.Ping(ctx, s.db); err != nil {
		checks["postgres"] = "down"
		ready = false
	}
	if err := cache.Ping(ctx, s.redis); err != nil {
		checks["redis"] = "down"
		ready = false
	}

	if s.storage != nil {
		if err := s.storage.Ping(ctx); err != nil {
			s.log.Warn("readyz_storage_soft_check", "error", err)
		}
	}
	if s.searchClient != nil && s.searchClient.Enabled() {
		if err := s.searchClient.Ping(ctx); err != nil {
			checks["meilisearch"] = "down"
			s.log.Warn("readyz_meilisearch_soft_check", "error", err)
		} else {
			checks["meilisearch"] = "ok"
		}
	}

	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}

	writeJSON(w, status, map[string]any{
		"status": state,
		"checks": checks,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func newCopilotHandler(q copilotmodule.Queries, ai copilotmodule.AI, log *slog.Logger) *copilotmodule.Handler {
	svc := copilotmodule.NewService(q)
	return copilotmodule.NewHandler(svc, copilotmodule.NewPlans(svc, copilotmodule.NewAIPlanner(ai), log))
}
