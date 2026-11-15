package methods

import (
	"context"
	"encoding/json"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/sources"
)

// previewLimit, source.preview'in döndürdüğü en fazla fatura (tamamı sayılır).
const previewLimit = 50

// RegisterSources Logo Tiger/GO ve Mikro yerel okumasını bağlar (issue #51,
// 14 §9, 15 §2). Salt okunur: yalnızca sources paketindeki önceden tanımlı,
// doğrulanmış SELECT sorguları çalışır.
//
// Okunan veri şimdilik sunucuya gönderilmez: portalda program verisini
// alacak uç yok (kurulu program senkronu ayrı bir iş). source.preview pilot
// mükellefte tablo/kolon adlarını ve tür eşlemesini doğrulamak içindir.
func RegisterSources(s *rpc.Server) {
	s.Handle("source.test", func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct {
			Config sources.Config `json:"config"`
		}
		if err := rpc.Decode(params, &p); err != nil {
			return nil, err
		}
		r, closeFn, err := openReader(p.Config)
		if err != nil {
			return nil, err
		}
		defer closeFn()
		return r.Test(ctx)
	})
	s.Handle("source.preview", func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct {
			Config sources.Config `json:"config"`
			From   string         `json:"from"`
			To     string         `json:"to"`
		}
		if err := rpc.Decode(params, &p); err != nil {
			return nil, err
		}
		from, err1 := time.Parse(time.DateOnly, p.From)
		to, err2 := time.Parse(time.DateOnly, p.To)
		if err1 != nil || err2 != nil || !from.Before(to) || to.Sub(from) > 92*24*time.Hour {
			return nil, rpc.Errorf(rpc.CodeInvalidParams, "from/to YYYY-AA-GG olmalı, from < to ve en çok 92 gün")
		}
		r, closeFn, err := openReader(p.Config)
		if err != nil {
			return nil, err
		}
		defer closeFn()
		b, err := r.ReadInvoices(ctx, from, to)
		if err != nil {
			return nil, err
		}
		return preview(b), nil
	})
}

func openReader(cfg sources.Config) (*sources.Reader, func(), error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	q, err := sources.Open(cfg)
	if err != nil {
		return nil, nil, err
	}
	r, err := sources.NewReader(cfg, q)
	if err != nil {
		_ = q.Close()
		return nil, nil, err
	}
	return r, func() { _ = q.Close() }, nil
}

type previewResult struct {
	Source      sources.Kind      `json:"source"`
	Total       int               `json:"total"`
	Skipped     int               `json:"skipped"`
	Truncated   bool              `json:"truncated"`
	ByDirection map[string]int    `json:"by_direction"`
	Warnings    map[string]int    `json:"warning_counts"`
	Batch       []sources.Warning `json:"batch_warnings"`
	Sample      []sources.Invoice `json:"sample"`
}

func preview(b *sources.InvoiceBatch) previewResult {
	out := previewResult{
		Source: b.Source, Total: len(b.Invoices), Skipped: b.Skipped, Truncated: b.Truncated,
		ByDirection: map[string]int{}, Warnings: map[string]int{}, Batch: b.Warnings,
	}
	for i, inv := range b.Invoices {
		out.ByDirection[string(inv.Direction)]++
		for _, w := range inv.Warnings {
			out.Warnings[w.Code]++
		}
		if i < previewLimit {
			out.Sample = append(out.Sample, inv)
		}
	}
	return out
}
