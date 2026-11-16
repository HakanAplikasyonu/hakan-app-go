package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
)

// Config holds process configuration loaded from the environment.
type Config struct {
	App        AppConfig
	Encryption EncryptionConfig
	HTTP       HTTPConfig
	DB         DBConfig
	Redis      RedisConfig
	Storage    StorageConfig
	CORS       CORSConfig
	SMTP       SMTPConfig
	Queue      QueueConfig
	Egress     EgressConfig
	Bulk       BulkConfig
	Centrifugo CentrifugoConfig
	Auth       AuthConfig
	JWT        JWTConfig
	Log        LogConfig
	VAPID      VAPIDConfig
	Expo       ExpoConfig
	Search     SearchConfig
	Sentry     SentryConfig
	Metrics    MetricsConfig
	Tracing    TracingConfig
	Tenancy    TenancyConfig
	AI         AIConfig
	Parasut    ParasutConfig
	Easytrade  EasytradeConfig
	KolayBi    KolayBiConfig
	Bank       BankConfig
	Privacy    PrivacyConfig
}

// PrivacyConfig: KVKK data subject requests (issue #193, docs/kvkk.md §7).
type PrivacyConfig struct {
	// AccountingRetentionYears is reported on erasure as how long accounting
	// records and the taxpayer identity are kept (PRIVACY_ACCOUNTING_RETENTION_YEARS,
	// default 5: VUK md. 253).
	AccountingRetentionYears int
	// ExportTTL is how long an approved export can be downloaded
	// (PRIVACY_EXPORT_TTL, default 168h).
	ExportTTL time.Duration
}

// ParasutConfig is the Paraşüt integrator application (docs/kaynak 10 §1.1):
// one registration for the whole platform; each taxpayer authorizes it.
type ParasutConfig struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// KolayBiConfig is the KolayBi API (docs/kaynak 11 §1.1): the channel is the
// integrator's, each taxpayer brings an api_key.
type KolayBiConfig struct {
	BaseURL string
	Channel string
}

// EasytradeConfig is the Easytrade Muhasebe API (docs/kaynak 09).
type EasytradeConfig struct {
	BaseURL string
}

// BankConfig is the bank web-service channel (docs/kaynak 20 §4–5). The
// fixed egress IPs and the technical contact are printed on the branch
// instruction the taxpayer signs; they never change once banks know them.
type BankConfig struct {
	EgressIPs   []string
	TechContact string
}

// TenancyConfig controls organizations (the multi-tenant shell). See
// docs/design/tenant-core.md.
// AIConfig configures the AI module (internal/modules/ai). The primary
// provider is Anthropic; the fallback is optional and used only when the
// primary call fails. Keys come from the environment only.
type AIConfig struct {
	Provider string
	APIKey   string
	BaseURL  string
	Model    string

	FallbackProvider string
	FallbackAPIKey   string
	FallbackBaseURL  string
	FallbackModel    string

	// Masking replaces VKN, TCKN, IBAN and known names before a request
	// leaves the server. Turning it off is for local debugging only.
	Masking bool
	// DisableProviderStore sends store=false to OpenAI-compatible providers
	// (no retention for evals / training).
	DisableProviderStore bool
	MaxTokens            int
	// ImageRedaction: "required" (default) blacks out VKN, TCKN, IBAN and
	// names in images with an OCR pre-pass before they are sent, and refuses
	// image calls when no OCR engine is found; "off" sends images as they
	// are (issue #169).
	ImageRedaction string
	// OCRTesseractPath is the tesseract binary (default: from PATH).
	OCRTesseractPath string
	// OCRLang is the tesseract language (default "tur").
	OCRLang string
}

type TenancyConfig struct {
	// Enabled mounts the /v1/tenant routes. Off = single-scope app.
	Enabled bool
	// SelfServe lets signed-in users create organizations themselves.
	SelfServe bool
	// CacheTTL is how long a resolved organization + membership stays in
	// Redis. Changes invalidate it immediately; the TTL only bounds memory.
	CacheTTL time.Duration
	// InviteTTL is how long an organization invitation link stays valid.
	InviteTTL time.Duration
}

// SentryConfig configures error reporting to a Sentry-protocol endpoint.
// An empty DSN disables reporting entirely.
// MetricsConfig configures the Prometheus listener (issue #179). It runs on
// its own port so it is never exposed through the public API proxy.
type MetricsConfig struct {
	// Addr is the API's metrics listener (METRICS_ADDR, "" = off).
	Addr string
	// WorkerAddr is the worker's (WORKER_METRICS_ADDR, "" = off).
	WorkerAddr string
	// Token, when set, is required as "Authorization: Bearer <token>".
	Token string
}

// TracingConfig configures OpenTelemetry tracing (issue #179). The OTLP
// exporter itself reads the standard OTEL_EXPORTER_OTLP_* variables.
type TracingConfig struct {
	// Endpoint is OTEL_EXPORTER_OTLP_ENDPOINT or
	// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT; empty = tracing off.
	Endpoint string
	// ServiceName is OTEL_SERVICE_NAME (default: APP_NAME).
	ServiceName string
	// SampleRatio is OTEL_TRACES_SAMPLER_ARG (0–1, default 0.1), used with a
	// parent-based ratio sampler.
	SampleRatio float64
}

type SentryConfig struct {
	DSN         string
	Environment string
	// Release defaults to the SDK's own detection (SENTRY_RELEASE, build info).
	Release string
	// TracesSampleRate is the share of HTTP requests sent as performance
	// transactions (0 = tracing off).
	TracesSampleRate float64
}

// AuthConfig holds NextAuth adapter integration settings.
type AuthConfig struct {
	AdapterSecret string
	WebAuthnRPID  string
	FrontendURL   string
	// ReviewAccounts maps a lower-cased email to a fixed sign-in code for
	// App Store / Play review (AUTH_REVIEW_ACCOUNTS=email:code,...). Empty = off.
	ReviewAccounts map[string]string
	// AppleNativeClientIDs are accepted id_token audiences for native Apple
	// sign-in (iOS bundle id). GoogleNativeClientIDs are the iOS/Android OAuth
	// client ids; the web client id from OAuth settings is accepted as well.
	AppleNativeClientIDs  []string
	GoogleNativeClientIDs []string
	// Sign in with Apple key (.p8) used to exchange native authorization codes
	// and revoke refresh tokens on account deletion. Optional.
	AppleTeamID     string
	AppleKeyID      string
	ApplePrivateKey string
	// QR sign-in location lookup. QRGeoIPDB is an optional MaxMind
	// GeoLite2-City .mmdb path (AUTH_QR_GEOIP_DB). QRTrustGeoHeaders trusts
	// edge geo headers (CF-IPCountry / CF-IPCity / CF-Region) and must only be
	// on when the edge (Cloudflare) overwrites them (AUTH_QR_TRUST_GEO_HEADERS).
	QRGeoIPDB         string
	QRTrustGeoHeaders bool
}

// VAPIDConfig holds Web Push keys (empty = push disabled).
type VAPIDConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// ExpoConfig holds the Expo Push Service settings (mobile push). The access
// token is optional: only needed with Expo "enhanced push security".
type ExpoConfig struct {
	AccessToken string
}

// JWTConfig holds access JWT and opaque refresh token settings.
type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

type AppConfig struct {
	Name string
	Env  string
}

// EncryptionConfig holds at-rest secret encryption material.
type EncryptionConfig struct {
	Key string
	// SecretsKeyring is the credential vault's KEK list ("id:base64,id2:base64").
	// Empty: the vault wraps data keys with Key (KEK id "app"). See
	// internal/platform/secrets and backend/docs/sir-kasasi.md.
	SecretsKeyring string
	// SecretsActiveKeyID picks the KEK for new records and rotation; empty
	// means the last SecretsKeyring entry (or "app").
	SecretsActiveKeyID string
	// FieldKey (FIELD_ENCRYPTION_KEY, base64 of 32 bytes) is the KEK of
	// field-level encryption (#168: raw bank / invoice payloads), KEK id
	// "field". It must differ from Key and the vault keys. FieldKeyring
	// (FIELD_ENCRYPTION_KEYRING, "id:base64,…") adds keys for rotation and
	// FieldActiveKeyID (FIELD_ENCRYPTION_ACTIVE_KEY_ID) picks the active one.
	// See backend/docs/alan-sifreleme.md.
	FieldKey         string
	FieldKeyring     string
	FieldActiveKeyID string
}

type HTTPConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	// TrustedProxies lists CIDRs / IPs whose X-Forwarded-For hops are believed
	// (TRUSTED_PROXIES, comma-separated). Empty = loopback + private ranges;
	// "none" = never trust forwarding headers. See platform/clientip.
	TrustedProxies []string
}

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
	MaxConns int32
	// RLSRuntime switches every pooled connection to hakan_rls_runtime
	// (migration 000074, issue #166): row level security then applies to all
	// application queries, not only to rls.WithScope. Off only as an
	// emergency switch (DB_RLS_RUNTIME=false).
	RLSRuntime bool
	// PlanCacheMode is the session's plan_cache_mode (DB_PLAN_CACHE_MODE,
	// issue #182). pgx prepares every query; after five runs Postgres may
	// switch to a generic plan that knows neither the LIMIT nor which
	// optional filters (sqlc.narg) are set, and then sorts whole lists.
	// "force_custom_plan" (default) plans with the real values each time;
	// "auto" restores the Postgres default.
	PlanCacheMode string
	// RLSRequestScope runs every tenant request in one row level security
	// transaction for its office (issue #166). Off only as an emergency
	// switch (DB_RLS_REQUEST_SCOPE=false).
	RLSRequestScope bool
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Name,
		c.SSLMode,
	)
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type StorageConfig struct {
	Driver    string // minio | s3 | local
	LocalPath string
	MaxBytes  int64
	MinIO     ObjectStoreConfig
	S3        ObjectStoreConfig
}

// ObjectStoreConfig holds S3-compatible credentials (MinIO or S3).
type ObjectStoreConfig struct {
	Endpoint      string
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
	UsePathStyle  bool
	PublicBaseURL string
}

type CORSConfig struct {
	AllowedOrigins []string
}

// SMTPConfig configures outbound email (MailHog in development).
type SMTPConfig struct {
	Driver   string
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	// Timeout bounds the whole send (dial + TLS + handshake + delivery). A
	// hung SMTP server must never block a worker or request indefinitely.
	Timeout time.Duration
	// TLSMode selects transport security: "auto" (implicit TLS on port 465,
	// otherwise STARTTLS when the server advertises it), "starttls" (require
	// STARTTLS), "implicit" (wrap the connection in TLS immediately), or
	// "none" (plaintext, e.g. MailHog in development).
	TLSMode string
	// TLSInsecureSkipVerify disables certificate verification. Intended only
	// for self-signed servers in development; validate() refuses it in prod.
	TLSInsecureSkipVerify bool
}

type LogConfig struct {
	Level  string
	Format string
}

// QueueConfig controls Asynq background jobs.
type QueueConfig struct {
	Enabled         bool
	Concurrency     int
	WorkerInProcess bool
	// WorkerPool is all | general | fixed-egress (issue #45).
	WorkerPool string
}

// EgressConfig is the fixed outbound IP setup (issue #45).
type EgressConfig struct {
	// ProxyURL routes fixed-egress calls through a forward proxy. Empty:
	// direct from the worker host.
	ProxyURL string
	// ExpectedIPs are the allowlisted public IPs to verify.
	ExpectedIPs []string
	// CheckURL answers with the caller's public IP as text.
	CheckURL string
	// CheckInterval is the period of the IP check.
	CheckInterval time.Duration
}

// BulkConfig controls bulk action engine thresholds.
type BulkConfig struct {
	SyncMax       int
	RollbackHours int
}

// CentrifugoConfig controls realtime pub/sub via Centrifugo.
type CentrifugoConfig struct {
	Enabled   bool
	APIURL    string
	APIKey    string
	TokenHMAC string
	WSURL     string
	TokenTTL  time.Duration
}

// SearchConfig controls Meilisearch-backed command palette indexing.
type SearchConfig struct {
	Enabled     bool
	Driver      string
	MeiliHost   string
	MeiliKey    string
	IndexPrefix string
}

// Load reads optional .env then environment variables into Config.
func Load() (Config, error) {
	_ = godotenv.Load("../.env", ".env")

	frontendURL := getEnv("PUBLIC_FRONTEND_URL", "http://localhost:3000")

	cfg := Config{
		App: AppConfig{
			Name: getEnv("APP_NAME", "api"),
			Env:  getEnv("APP_ENV", "development"),
		},
		Privacy: PrivacyConfig{
			AccountingRetentionYears: getInt("PRIVACY_ACCOUNTING_RETENTION_YEARS", 5),
			ExportTTL:                getDuration("PRIVACY_EXPORT_TTL", 7*24*time.Hour),
		},
		Encryption: EncryptionConfig{
			Key:                getEnv("APP_ENCRYPTION_KEY", defaultEncryptionKey),
			SecretsKeyring:     getEnv("SECRETS_KEYRING", ""),
			SecretsActiveKeyID: getEnv("SECRETS_ACTIVE_KEY_ID", ""),
			FieldKey:           getEnv("FIELD_ENCRYPTION_KEY", defaultFieldEncryptionKey),
			FieldKeyring:       getEnv("FIELD_ENCRYPTION_KEYRING", ""),
			FieldActiveKeyID:   getEnv("FIELD_ENCRYPTION_ACTIVE_KEY_ID", ""),
		},
		HTTP: HTTPConfig{
			Addr:         getEnv("APP_HTTP_ADDR", ":8080"),
			ReadTimeout:  getDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),

			TrustedProxies: splitCSV(getEnv("TRUSTED_PROXIES", "")),
		},
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "127.0.0.1"),
			Port:     getInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "app"),
			Password: getEnv("DB_PASSWORD", defaultDBPassword),
			Name:     getEnv("DB_NAME", "app"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
			MaxConns: intconv.Int32(getInt("DB_MAX_CONNS", 20)),
			// RLS for every query (issue #166); false is the emergency switch.
			RLSRuntime:    getBool("DB_RLS_RUNTIME", true),
			PlanCacheMode: strings.TrimSpace(getEnv("DB_PLAN_CACHE_MODE", "force_custom_plan")),
			// Tenant requests in one scoped transaction (issue #166).
			RLSRequestScope: getBool("DB_RLS_REQUEST_SCOPE", true),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getInt("REDIS_DB", 0),
		},
		Storage: loadStorageConfig(),
		CORS: CORSConfig{
			AllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		},
		SMTP: SMTPConfig{
			Driver:                strings.ToLower(getEnv("MAIL_DRIVER", "smtp")),
			Host:                  getEnv("SMTP_HOST", "127.0.0.1"),
			Port:                  getInt("SMTP_PORT", 1025),
			Username:              getEnv("SMTP_USERNAME", ""),
			Password:              getEnv("SMTP_PASSWORD", ""),
			From:                  firstNonEmpty(getEnv("MAIL_FROM", ""), getEnv("SMTP_FROM", "noreply@localhost")),
			FromName:              getEnv("MAIL_FROM_NAME", "App"),
			Timeout:               getDuration("SMTP_TIMEOUT", 10*time.Second),
			TLSMode:               strings.ToLower(getEnv("SMTP_TLS", "auto")),
			TLSInsecureSkipVerify: getBool("SMTP_TLS_INSECURE_SKIP_VERIFY", false),
		},
		Queue: QueueConfig{
			Enabled:         getBool("QUEUE_ENABLED", true),
			Concurrency:     getInt("QUEUE_CONCURRENCY", 10),
			WorkerInProcess: getBool("QUEUE_WORKER_INPROCESS", true),
			WorkerPool:      strings.ToLower(strings.TrimSpace(getEnv("WORKER_POOL", "all"))),
		},
		Egress: EgressConfig{
			ProxyURL:      getEnv("EGRESS_PROXY_URL", ""),
			ExpectedIPs:   splitCSV(getEnv("EGRESS_EXPECTED_IPS", "")),
			CheckURL:      getEnv("EGRESS_IP_CHECK_URL", "https://api.ipify.org"),
			CheckInterval: getDuration("EGRESS_CHECK_INTERVAL", 15*time.Minute),
		},
		Bulk: BulkConfig{
			SyncMax:       getInt("BULK_SYNC_MAX", 50),
			RollbackHours: getInt("BULK_ROLLBACK_HOURS", 24),
		},
		Centrifugo: CentrifugoConfig{
			Enabled:   getBool("CENTRIFUGO_ENABLED", true),
			APIURL:    getEnv("CENTRIFUGO_API_URL", "http://127.0.0.1:8000"),
			APIKey:    getEnv("CENTRIFUGO_API_KEY", defaultCentrifugoAPIKey),
			TokenHMAC: getEnv("CENTRIFUGO_TOKEN_HMAC_SECRET", defaultCentrifugoTokenHMAC),
			WSURL:     getEnv("CENTRIFUGO_WS_URL", "ws://127.0.0.1:8000/connection/websocket"),
			TokenTTL:  getDuration("CENTRIFUGO_TOKEN_TTL", time.Hour),
		},
		Auth: AuthConfig{
			AdapterSecret: getEnv("AUTH_ADAPTER_SECRET", defaultAdapterSecret),
			WebAuthnRPID:  getEnv("AUTH_WEBAUTHN_RP_ID", hostOf(frontendURL, "localhost")),
			FrontendURL:   frontendURL,

			ReviewAccounts:        parseReviewAccounts(getEnv("AUTH_REVIEW_ACCOUNTS", "")),
			AppleNativeClientIDs:  splitCSV(getEnv("AUTH_APPLE_NATIVE_CLIENT_IDS", "")),
			GoogleNativeClientIDs: splitCSV(getEnv("AUTH_GOOGLE_NATIVE_CLIENT_IDS", "")),
			AppleTeamID:           getEnv("AUTH_APPLE_TEAM_ID", ""),
			AppleKeyID:            getEnv("AUTH_APPLE_KEY_ID", ""),
			ApplePrivateKey:       getEnv("AUTH_APPLE_PRIVATE_KEY", ""),
			QRGeoIPDB:             getEnv("AUTH_QR_GEOIP_DB", ""),
			QRTrustGeoHeaders:     getBool("AUTH_QR_TRUST_GEO_HEADERS", false),
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", defaultJWTAccessSecret),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", defaultJWTRefreshSecret),
			AccessTTL:     getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL:    getDuration("JWT_REFRESH_TTL", 168*time.Hour),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "debug"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		VAPID: VAPIDConfig{
			PublicKey:  getEnv("VAPID_PUBLIC_KEY", ""),
			PrivateKey: getEnv("VAPID_PRIVATE_KEY", ""),
			Subject:    getEnv("VAPID_SUBJECT", "mailto:noreply@example.com"),
		},
		Expo: ExpoConfig{
			AccessToken: getEnv("EXPO_ACCESS_TOKEN", ""),
		},
		Search: SearchConfig{
			Enabled:     getBool("SEARCH_ENABLED", true),
			Driver:      strings.ToLower(getEnv("SEARCH_DRIVER", "meilisearch")),
			MeiliHost:   getEnv("MEILI_HOST", "http://127.0.0.1:7700"),
			MeiliKey:    getEnv("MEILI_MASTER_KEY", defaultMeiliMasterKey),
			IndexPrefix: getEnv("MEILI_INDEX_PREFIX", "app"),
		},
		Metrics: MetricsConfig{
			Addr:       getEnv("METRICS_ADDR", ":9091"),
			WorkerAddr: getEnv("WORKER_METRICS_ADDR", ":9092"),
			Token:      getEnv("METRICS_TOKEN", ""),
		},
		Tracing: TracingConfig{
			Endpoint:    strings.TrimSpace(getEnv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""))),
			ServiceName: getEnv("OTEL_SERVICE_NAME", getEnv("APP_NAME", "app")),
			SampleRatio: clampRate(getFloat("OTEL_TRACES_SAMPLER_ARG", 0.1)),
		},
		Sentry: SentryConfig{
			DSN:              strings.TrimSpace(getEnv("SENTRY_DSN", "")),
			Environment:      getEnv("SENTRY_ENVIRONMENT", getEnv("APP_ENV", "development")),
			Release:          getEnv("SENTRY_RELEASE", ""),
			TracesSampleRate: clampRate(getFloat("SENTRY_TRACES_SAMPLE_RATE", 0)),
		},
		Tenancy: TenancyConfig{
			Enabled:   getBool("TENANCY_ENABLED", false),
			SelfServe: getBool("TENANCY_SELF_SERVE", false),
			CacheTTL:  getDuration("TENANCY_CACHE_TTL", 60*time.Second),
			InviteTTL: getDuration("TENANCY_INVITE_TTL", 168*time.Hour),
		},
		KolayBi:   KolayBiConfig{BaseURL: getEnv("KOLAYBI_BASE_URL", "https://ofis-api.kolaybi.com"), Channel: getEnv("KOLAYBI_CHANNEL", "")},
		Easytrade: EasytradeConfig{BaseURL: getEnv("EASYTRADE_API_BASE_URL", "https://pro.easytradetr.com/api")},
		Parasut: ParasutConfig{
			BaseURL:      getEnv("PARASUT_BASE_URL", "https://api.parasut.com"),
			ClientID:     getEnv("PARASUT_CLIENT_ID", ""),
			ClientSecret: getEnv("PARASUT_CLIENT_SECRET", ""),
			RedirectURL:  getEnv("PARASUT_REDIRECT_URL", ""),
		},
		Bank: BankConfig{
			EgressIPs:   splitList(getEnv("BANK_EGRESS_IPS", "")),
			TechContact: getEnv("BANK_TECH_CONTACT", ""),
		},
		AI: AIConfig{
			Provider:             getEnv("AI_PROVIDER", "anthropic"),
			APIKey:               getEnv("AI_API_KEY", ""),
			BaseURL:              getEnv("AI_BASE_URL", ""),
			Model:                getEnv("AI_MODEL", "claude-opus-5-5"),
			FallbackProvider:     getEnv("AI_FALLBACK_PROVIDER", ""),
			FallbackAPIKey:       getEnv("AI_FALLBACK_API_KEY", ""),
			FallbackBaseURL:      getEnv("AI_FALLBACK_BASE_URL", ""),
			FallbackModel:        getEnv("AI_FALLBACK_MODEL", ""),
			Masking:              getBool("AI_MASKING", true),
			DisableProviderStore: getBool("AI_DISABLE_PROVIDER_STORE", true),
			MaxTokens:            getInt("AI_MAX_TOKENS", 4096),
			ImageRedaction:       strings.ToLower(strings.TrimSpace(getEnv("AI_IMAGE_REDACTION", "required"))),
			OCRTesseractPath:     getEnv("AI_OCR_TESSERACT_PATH", "tesseract"),
			OCRLang:              getEnv("AI_OCR_LANG", "tur"),
		},
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// validate checks the loaded config. Secret defaults are refused outside
// development/test by secretGuard (guard.go), driven by deploy/env/schema.
func (c Config) validate() error {
	if err := validateAppEnv(c.App.Env); err != nil {
		return err
	}
	if c.App.Name == "" {
		return fmt.Errorf("config: APP_NAME is required")
	}
	if c.HTTP.Addr == "" {
		return fmt.Errorf("config: APP_HTTP_ADDR is required")
	}
	if c.DB.Host == "" || c.DB.User == "" || c.DB.Name == "" {
		return fmt.Errorf("config: database settings are incomplete")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("config: REDIS_ADDR is required")
	}
	switch c.Queue.WorkerPool {
	case "", "all", "general", "fixed-egress":
	default:
		return fmt.Errorf("config: WORKER_POOL must be all, general or fixed-egress")
	}
	if c.Queue.Concurrency <= 0 {
		return fmt.Errorf("config: QUEUE_CONCURRENCY must be positive")
	}
	if c.JWT.AccessTTL <= 0 || c.JWT.RefreshTTL <= 0 {
		return fmt.Errorf("config: JWT TTLs must be positive")
	}
	if err := c.secretGuard(); err != nil {
		return err
	}
	if !c.App.IsLocal() && len(c.JWT.AccessSecret) < 32 {
		return fmt.Errorf("config: JWT_ACCESS_SECRET must be at least 32 characters outside development")
	}
	if c.JWT.AccessSecret == "" {
		return fmt.Errorf("config: JWT_ACCESS_SECRET is required")
	}
	if c.Search.Enabled {
		if c.Search.MeiliHost == "" {
			return fmt.Errorf("config: MEILI_HOST is required when SEARCH_ENABLED=true")
		}
		if c.Search.IndexPrefix == "" {
			return fmt.Errorf("config: MEILI_INDEX_PREFIX is required when SEARCH_ENABLED=true")
		}
	}
	if c.Tenancy.Enabled && c.Tenancy.InviteTTL <= 0 {
		return fmt.Errorf("config: TENANCY_INVITE_TTL must be positive")
	}
	switch c.SMTP.TLSMode {
	case "", "auto", "starttls", "implicit", "none":
	default:
		return fmt.Errorf("config: SMTP_TLS must be one of auto|starttls|implicit|none")
	}
	if c.App.Env != "development" && c.App.Env != "test" && c.SMTP.TLSInsecureSkipVerify {
		return fmt.Errorf("config: SMTP_TLS_INSECURE_SKIP_VERIFY must not be set outside development")
	}
	switch c.DB.PlanCacheMode {
	case "", "auto", "force_custom_plan", "force_generic_plan":
	default:
		return fmt.Errorf("config: DB_PLAN_CACHE_MODE must be auto, force_custom_plan or force_generic_plan")
	}
	switch c.AI.ImageRedaction {
	case "", "required", "off":
	default:
		return fmt.Errorf("config: AI_IMAGE_REDACTION must be required or off")
	}
	return nil
}

func loadStorageConfig() StorageConfig {
	driver := strings.ToLower(getEnv("STORAGE_DRIVER", "minio"))
	s3Endpoint := getEnv("S3_ENDPOINT", "http://127.0.0.1:9000")
	s3Bucket := getEnv("S3_BUCKET", "app")
	s3Access := getEnv("S3_ACCESS_KEY", defaultS3AccessKey)
	s3Secret := getEnv("S3_SECRET_KEY", defaultS3SecretKey)
	s3Region := getEnv("S3_REGION", "us-east-1")
	s3PathStyle := getBool("S3_USE_PATH_STYLE", true)
	s3Public := getEnv("S3_PUBLIC_BASE_URL", "")

	minioEndpoint := firstNonEmpty(getEnv("MINIO_ENDPOINT", ""), s3Endpoint)
	minioBucket := firstNonEmpty(getEnv("MINIO_BUCKET", ""), s3Bucket)
	minioAccess := firstNonEmpty(getEnv("MINIO_ACCESS_KEY", ""), s3Access)
	minioSecret := firstNonEmpty(getEnv("MINIO_SECRET_KEY", ""), s3Secret)
	minioRegion := firstNonEmpty(getEnv("MINIO_REGION", ""), s3Region)
	minioPathStyle := s3PathStyle
	if os.Getenv("MINIO_USE_PATH_STYLE") != "" {
		minioPathStyle = getBool("MINIO_USE_PATH_STYLE", true)
	}
	minioPublic := firstNonEmpty(getEnv("MINIO_PUBLIC_BASE_URL", ""), s3Public)
	if minioPublic == "" {
		minioPublic = strings.TrimRight(minioEndpoint, "/") + "/" + minioBucket
	}
	if s3Public == "" {
		s3Public = strings.TrimRight(s3Endpoint, "/") + "/" + s3Bucket
	}

	return StorageConfig{
		Driver:    driver,
		LocalPath: getEnv("STORAGE_LOCAL_PATH", "./storage/objects"),
		MaxBytes:  getInt64("STORAGE_MAX_BYTES", 10<<20),
		MinIO: ObjectStoreConfig{
			Endpoint:      minioEndpoint,
			Region:        minioRegion,
			Bucket:        minioBucket,
			AccessKey:     minioAccess,
			SecretKey:     minioSecret,
			UsePathStyle:  minioPathStyle,
			PublicBaseURL: minioPublic,
		},
		S3: ObjectStoreConfig{
			Endpoint:      s3Endpoint,
			Region:        s3Region,
			Bucket:        s3Bucket,
			AccessKey:     s3Access,
			SecretKey:     s3Secret,
			UsePathStyle:  s3PathStyle,
			PublicBaseURL: s3Public,
		},
	}
}

// hostOf returns the hostname of rawURL (no port), or fallback when it has none.
func hostOf(rawURL, fallback string) string {
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return fallback
	}
	return f
}

func clampRate(v float64) float64 {
	if v != v || v < 0 { // NaN or negative
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseReviewAccounts parses "a@x.com:123456,b@y.com:654321". Entries without
// an email or a code of at least 6 characters are ignored.
func parseReviewAccounts(raw string) map[string]string {
	out := map[string]string{}
	for _, item := range splitCSV(raw) {
		email, code, ok := strings.Cut(item, ":")
		email = strings.ToLower(strings.TrimSpace(email))
		code = strings.TrimSpace(code)
		if !ok || email == "" || !strings.Contains(email, "@") || len(code) < 6 {
			continue
		}
		out[email] = code
	}
	return out
}

// splitList reads a comma-separated list, dropping empty items.
func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
