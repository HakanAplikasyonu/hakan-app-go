// Command receipt-eval measures receipt OCR field accuracy (issue #23
// acceptance: "20 örnek fişte alan doğruluğu ölçülüyor ve raporlanıyor").
//
// The sample directory holds image files (jpg, png, webp) and, next to each,
// a JSON file with the same base name and the ground truth:
//
//	{"kind": "fuel_receipt", "fields": {"date": "14.10.2026", "total_amount": "1.250,40", "vehicle_plate": "34 ABC 123"}}
//
// It reads every image with the configured AI provider (AI_* env, see
// .env.example) and prints a Markdown report per field. Calls are billed to
// no office; usage is not recorded.
//
//	go run ./cmd/receipt-eval -dir ./testdata/receipts > report.md
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	aimodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
)

func main() {
	dir := flag.String("dir", "", "directory with sample images and their .json ground truth")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "usage: receipt-eval -dir <samples>")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	ai := aimodule.NewService(cfg.AI, nil, slog.Default())
	if !ai.Enabled() {
		fmt.Fprintln(os.Stderr, "AI is not configured (AI_API_KEY)")
		os.Exit(1)
	}
	expected, images, err := load(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ex := extract.New(ai)
	results := make([]extract.Result, len(expected))
	for i, img := range images {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		// OrganizationID 1 only satisfies the service guard; the store is nil, nothing is recorded.
		r, err := ex.Read(ctx, extract.Request{OrganizationID: 1, MediaType: http.DetectContentType(img), Image: img})
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", expected[i].Name, err)
		}
		results[i] = r
	}
	fmt.Println("# Fiş OCR doğruluk raporu")
	fmt.Println()
	fmt.Println(extract.Score(expected, results, usecase.ReviewBelow).Markdown())
}

func load(dir string) ([]extract.Expected, [][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	var names []string
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var exp []extract.Expected
	var imgs [][]byte
	for _, n := range names {
		img, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, nil, err
		}
		raw, err := os.ReadFile(filepath.Join(dir, strings.TrimSuffix(n, filepath.Ext(n))+".json"))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: ground truth: %w", n, err)
		}
		var e extract.Expected
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", n, err)
		}
		e.Name = n
		exp = append(exp, e)
		imgs = append(imgs, img)
	}
	if len(exp) == 0 {
		return nil, nil, fmt.Errorf("no images in %s", dir)
	}
	return exp, imgs, nil
}
