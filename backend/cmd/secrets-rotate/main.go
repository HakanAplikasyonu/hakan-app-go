// Command secrets-rotate re-wraps every vault data key with the active KEK
// (SECRETS_ACTIVE_KEY_ID). Ciphertexts are untouched; the run is idempotent.
//
// Rotation, step by step (backend/docs/sir-kasasi.md):
//
//  1. Add the new key to SECRETS_KEYRING ("old:…,new:…") and set
//     SECRETS_ACTIVE_KEY_ID=new; redeploy (new records use "new").
//  2. Run this command (production image: /app/secrets-rotate).
//  3. When it reports no records left on the old key, drop the old key.
//
// It prints counts only, never key material or plaintext.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
)

func main() {
	batch := flag.Int("batch", 200, "records per batch")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	kr, err := secrets.ParseKeyring(cfg.Encryption.SecretsKeyring, cfg.Encryption.SecretsActiveKeyID, cfg.Encryption.Key)
	if err != nil {
		fatal("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	rep, err := secrets.New(kr, secrets.NewPGStore(db.New(pool))).Rotate(ctx, int32(*batch)) //nolint:gosec // flag value, small
	if err != nil {
		fatal("rotate: %v", err)
	}
	fmt.Printf("active key: %s\nre-wrapped: %d\nfailed (key not in keyring): %d\n", rep.ActiveKEK, rep.Rewrapped, rep.Failed)
	ids := make([]string, 0, len(rep.Remaining))
	for id := range rep.Remaining {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		fmt.Printf("records on %s: %d\n", id, rep.Remaining[id])
	}
	if rep.Failed > 0 {
		os.Exit(1)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "secrets-rotate: "+format+"\n", args...)
	os.Exit(1)
}
