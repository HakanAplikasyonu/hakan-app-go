// Command hakan-sidecar, masaüstü uygulamasının (Tauri kabuğu) yanında
// çalışan Go sürecidir (docs/01 §4.2, D5). Kabuk onu Tauri sidecar olarak
// başlatır ve stdin/stdout üzerinden satır tabanlı JSON ile konuşur
// (bkz. rpc paketi). İş mantığı burada; Rust kabuk ince kalır.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/agent"
	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/methods"
	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// Derlemede -ldflags "-X main.version=… -X main.commit=…" ile doldurulur.
// defaultBaseURL, sürüm derlemesinde HAKAN_API_BASE_URL'den gelir
// (scripts/build-sidecar.mjs); kullanıcı adres girmediyse önerilir.
var (
	version        = "dev"
	commit         = "none"
	defaultBaseURL = ""
)

func main() {
	showVersion := flag.Bool("version", false, "sürümü yaz ve çık")
	dataDir := flag.String("data-dir", "", "ajan durum dizini (kabuk uygulama veri dizinini verir)")
	flag.Parse()
	info := methods.NewInfo(version, commit)
	if *showVersion {
		fmt.Printf("hakan-sidecar %s (%s) %s/%s\n", info.Version, info.Commit, info.OS, info.Arch)
		return
	}

	// stdout protokole ayrıldı; günlükler stderr'e.
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := rpc.NewServer(os.Stdout)
	methods.Register(srv, info, nil)

	dir := *dataDir
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			log.Error("veri dizini bulunamadı", "err", err)
			os.Exit(1)
		}
		dir = filepath.Join(base, "app.hakan.desktop")
	}
	rt, err := agent.NewRuntime(dir, version, func(name string, data any) {
		if err := srv.Emit(name, data); err != nil {
			log.Warn("olay yazılamadı", "event", name, "err", err)
		}
	})
	if err != nil {
		log.Error("ajan durumu yüklenemedi", "err", err)
		os.Exit(1)
	}
	defer rt.Stop()
	rt.DefaultBaseURL = defaultBaseURL
	methods.RegisterAgent(srv, rt)
	methods.RegisterSources(srv)
	if err := srv.Emit("ready", info); err != nil {
		log.Error("ready olayı yazılamadı", "err", err)
		os.Exit(1)
	}
	if err := srv.Serve(ctx, os.Stdin); err != nil {
		log.Error("protokol okunamadı", "err", err)
		os.Exit(1)
	}
}
