module github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar

go 1.26.11

require (
	github.com/HakanAplikasyonu/hakan-app-go/backend v0.0.0
	github.com/furkanmeclis/go-ubltr v0.1.0
)

require (
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/lestrrat-go/helium v0.7.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

// Sidecar, backend ile aynı Go paketlerini kullanır (docs/01 §4.2, D5).
// Monorepo içinden çözülür; ayrı sürümlenmez.
replace github.com/HakanAplikasyonu/hakan-app-go/backend => ../../backend
