module github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar

go 1.26.11

require (
	github.com/HakanAplikasyonu/hakan-app-go/backend v0.0.0
	github.com/chromedp/cdproto v0.160.12
	github.com/chromedp/chromedp v0.24.0
	github.com/furkanmeclis/go-ubltr v0.1.0
	github.com/microsoft/go-mssqldb v1.11.2
	github.com/shopspring/decimal v1.6.0
	golang.org/x/text v0.44.0
)

require (
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260213210345-44df1a37e875 // indirect
	github.com/golang-sql/civil v0.0.0-20220223132316-b832511892a9 // indirect
	github.com/golang-sql/sqlexp v0.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/lestrrat-go/helium v0.7.0 // indirect
	golang.org/x/crypto v0.59.0 // indirect
)

// Sidecar, backend ile aynı Go paketlerini kullanır (docs/01 §4.2, D5).
// Monorepo içinden çözülür; ayrı sürümlenmez.
replace github.com/HakanAplikasyonu/hakan-app-go/backend => ../../backend
