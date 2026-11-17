// Command mailpreview renders every outgoing email with sample data
// (issue #467): the notification templates stored in the database, the
// reconciliation letter and the taxpayer report with their PDFs.
//
// Usage (repo root):
//
//	make mail-preview                          # files under backend/tmp/mail-preview
//	make mail-preview ARGS="-send 127.0.0.1:1025"   # also deliver to MailHog
//
// Open backend/tmp/mail-preview/index.html, or MailHog at http://127.0.0.1:8025.
// -send only accepts a local catcher (localhost, 127.0.0.1, mailhog): this
// tool never mails a real address. It refuses APP_ENV=production.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/mailpreview"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
)

func main() {
	out := flag.String("out", "tmp/mail-preview", "directory for the .html, .txt and .eml files")
	send := flag.String("send", "", "host:port of a local SMTP catcher (MailHog) to deliver the previews to")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	if cfg.App.Env == "production" {
		fatal("refusing to run with APP_ENV=production")
	}
	if *send != "" && !mailpreview.LocalSMTP(*send) {
		fatal("-send %s: only a local catcher is allowed (localhost, 127.0.0.1, mailhog)", *send)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()
	tpls, err := mailpreview.LoadTemplates(ctx, pool)
	if err != nil {
		fatal("templates: %v", err)
	}
	samples, err := mailpreview.Build(tpls, cfg.Auth.FrontendURL, time.Now())
	if err != nil {
		fatal("render: %v", err)
	}
	if err := mailpreview.Write(*out, samples); err != nil {
		fatal("write: %v", err)
	}
	fmt.Printf("%d e-posta yazıldı: %s/index.html\n", len(samples), *out)

	if *send != "" {
		host, portStr, _ := net.SplitHostPort(*send)
		port, _ := strconv.Atoi(portStr)
		smtp := cfg.SMTP
		smtp.Host, smtp.Port, smtp.Username, smtp.Password, smtp.TLSMode = host, port, "", "", "none"
		if err := mailpreview.Send(ctx, *send, mail.NewSMTPSender(smtp), samples); err != nil {
			fatal("send: %v", err)
		}
		fmt.Printf("%d e-posta %s adresindeki yerel sunucuya gönderildi\n", len(samples), *send)
	}
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "mailpreview: "+format+"\n", a...)
	os.Exit(1)
}
