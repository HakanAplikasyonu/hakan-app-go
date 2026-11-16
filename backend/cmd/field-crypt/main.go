// Command field-crypt migrates field-level encrypted columns (#168: raw bank /
// invoice payloads) between plaintext and sealed form and re-wraps their data
// keys after a key rotation. Every mode is idempotent and resumable.
//
//	-mode status   counts per column and KEK id ("plaintext" = not sealed yet)
//	-mode encrypt  seal existing plaintext values (forward migration)
//	-mode decrypt  turn sealed values back into plaintext (reverse migration)
//	-mode rotate   re-wrap data keys with FIELD_ENCRYPTION_ACTIVE_KEY_ID
//
// -columns limits the run ("bank_transactions,invoices.raw_payload").
// Steps: backend/docs/alan-sifreleme.md. It prints counts only, never key
// material or payloads. Production image: /app/field-crypt.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/fieldcrypt"
)

func main() {
	mode := flag.String("mode", "status", "status | encrypt | decrypt | rotate")
	batch := flag.Int("batch", 500, "rows per transaction")
	columns := flag.String("columns", "", "comma-separated tables or table.column (default: all)")
	timeout := flag.Duration("timeout", 2*time.Hour, "overall time limit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	if *mode == "status" {
		counts, err := fieldcrypt.Status(ctx, pool)
		if err != nil {
			fatal("%v", err)
		}
		for _, c := range counts {
			kek := c.KEK
			if kek == "" {
				kek = "plaintext"
			}
			fmt.Printf("%-40s %-12s %d\n", c.Column, kek, c.Count)
		}
		return
	}

	fc, err := fieldcrypt.FromConfig(cfg.Encryption)
	if err != nil {
		fatal("%v", err)
	}
	var only []string
	for _, c := range strings.Split(*columns, ",") {
		if c = strings.TrimSpace(c); c != "" {
			only = append(only, c)
		}
	}
	rep, err := fieldcrypt.Run(ctx, pool, fc, fieldcrypt.Mode(*mode), *batch, only)
	fmt.Printf("mode: %s\nactive key: %s\n", rep.Mode, rep.Active)
	for _, c := range rep.Columns {
		fmt.Printf("%-40s changed=%d skipped=%d failed=%d\n", c.Column, c.Changed, c.Skipped, c.Failed)
	}
	if err != nil {
		fatal("%v", err)
	}
	if rep.Failed() > 0 {
		os.Exit(1)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "field-crypt: "+format+"\n", args...)
	os.Exit(1)
}
