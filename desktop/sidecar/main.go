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
	"syscall"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/methods"
	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// Derlemede -ldflags "-X main.version=… -X main.commit=…" ile doldurulur.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	showVersion := flag.Bool("version", false, "sürümü yaz ve çık")
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
	if err := srv.Emit("ready", info); err != nil {
		log.Error("ready olayı yazılamadı", "err", err)
		os.Exit(1)
	}
	if err := srv.Serve(ctx, os.Stdin); err != nil {
		log.Error("protokol okunamadı", "err", err)
		os.Exit(1)
	}
}
