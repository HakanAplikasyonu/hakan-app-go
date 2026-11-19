package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	accountrulesmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules"
	accountrulesrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/repository"
	accountrulesusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/usecase"
	reviewrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/queue"
)

// bigAccountSuggestPerClient: içinde bulunulan ayın yeni gelen faturalarından
// kaçı hesap kodu önerisiyle incelemeye düşer. Gerçek ofiste ay sürerken
// birkaç alış faturası kodlanmayı bekler; yüzlercesi değil.
const bigAccountSuggestPerClient = 4

// bigAccountSuggest, son dönemin (ay sürüyor) gelen faturalarından birkaçı
// için uygulamanın kendi hesap kodu önerisini çalıştırır (#569): UBL
// yüklemesinden sonra çalışan işle aynı yol. Kural ya da hafıza güçlü
// olanlar kendiliğinden uygulanır, gerisi İnceleme kuyruğunda "Kodu düzelt"
// ile karar bekler.
func bigAccountSuggest(ctx context.Context, pool *pgxpool.Pool, w *bigWorld, o *bigOffice, ms []*clientModel, cnt *bigCounts) error {
	q := db.New(pool)
	review := reviewusecase.New(reviewrepository.New(pool, q)).
		Register(accountrulesusecase.Kind, accountrulesrepository.Applier{})
	svc := accountrulesusecase.New(accountrulesrepository.New(pool, q), review)
	run := accountrulesmodule.RunAutoSuggest(pool, svc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p := w.periods[len(w.periods)-1]
	return parallelClients(ms, func(_ int, m *clientModel) error {
		var ids []uuid.UUID
		err := rls.System(ctx, pool, "demo-seed: invoices to suggest", func(ctx context.Context, tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `
				SELECT uuid FROM invoices
				WHERE taxpayer_id = $1 AND period = $2 AND direction = 'incoming' AND has_items
				ORDER BY issue_date DESC, id DESC LIMIT $3`, m.c.taxpayerID, p, bigAccountSuggestPerClient)
			if err != nil {
				return err
			}
			ids, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
			return err
		})
		if err != nil {
			return fmt.Errorf("invoices to suggest: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		return run(ctx, queue.AccountSuggestPayload{
			OrganizationID: o.orgID, EngagementID: m.c.engagementID, TaxpayerID: m.c.taxpayerID, Invoices: ids,
		})
	})
}
