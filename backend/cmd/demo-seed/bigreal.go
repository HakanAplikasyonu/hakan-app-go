package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// bigRealLife adds the office's everyday mistakes to the big demo (#568), so
// every warning points at a real cause the user can open and fix:
//   - mükerrer fatura: the supplier sent this month's invoice twice under a
//     second number (same party, date and amount); the copy carries
//     POSSIBLE_DUPLICATE with the original as candidate, like the upload
//     channel raises it;
//   - yanlış hesap: a personel booked an expense line of this month's draft
//     on the parent account (770 instead of 770.01); the journal check shows
//     it and the line can be corrected;
//   - stajyer işi: drafts a stajyer prepared wait for the müşavir's approval.
func bigRealLife(ctx context.Context, pool *pgxpool.Pool, w *bigWorld, o *bigOffice, ms []*clientModel, cnt *bigCounts) error {
	last := w.periods[len(w.periods)-1]
	var problem, engs []int64
	for _, m := range ms {
		engs = append(engs, m.c.engagementID)
		if m.c.Problem {
			problem = append(problem, m.c.engagementID)
		}
	}
	trainees := o.staffWithTitle("Stajyer")
	return rls.System(ctx, pool, "demo-seed: big real life", func(ctx context.Context, tx pgx.Tx) error {
		n, err := duplicateInvoices(ctx, tx, problem, last)
		if err != nil {
			return fmt.Errorf("duplicates: %w", err)
		}
		cnt.add("invoices (mükerrer)", n)

		tag, err := tx.Exec(ctx, `
			UPDATE journal_draft_lines l SET account_code = split_part(l.account_code, '.', 1)
			FROM (
				SELECT DISTINCT ON (d.engagement_id) x.id
				FROM journal_drafts d JOIN journal_draft_lines x ON x.journal_draft_id = d.id
				WHERE d.engagement_id = ANY($1::bigint[]) AND d.period = $2 AND d.status = 'draft'
				  AND x.account_code LIKE '7%.%' AND x.debit > 0
				ORDER BY d.engagement_id, d.entry_no DESC, x.line_no
			) pick
			WHERE l.id = pick.id`, problem, last)
		if err != nil {
			return fmt.Errorf("wrong account: %w", err)
		}
		cnt.add("journal_draft_lines (ana hesap)", int(tag.RowsAffected()))

		if len(trainees) > 0 {
			tag, err = tx.Exec(ctx, `
				UPDATE journal_drafts d SET created_by = ($3::bigint[])[1 + d.id % array_length($3::bigint[], 1)]
				WHERE d.engagement_id = ANY($1::bigint[]) AND d.period = $2 AND d.status = 'draft' AND d.id % 3 = 0`,
				engs, last, trainees)
			if err != nil {
				return fmt.Errorf("trainee drafts: %w", err)
			}
			cnt.add("journal_drafts (stajyer)", int(tag.RowsAffected()))
		}
		return nil
	})
}

// staffWithTitle returns the user ids of the office members with title.
func (o *bigOffice) staffWithTitle(title string) []int64 {
	var out []int64
	for i, m := range o.def.Staff(o.idx) {
		if m.Title == title && i < len(o.staff) {
			out = append(out, o.staff[i])
		}
	}
	return out
}

// duplicateInvoices copies one incoming invoice of period per engagement
// (lines and VAT summary too) under a second number and raises
// POSSIBLE_DUPLICATE on the copy.
func duplicateInvoices(ctx context.Context, tx pgx.Tx, engs []int64, period string) (int, error) {
	if len(engs) == 0 {
		return 0, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (i.taxpayer_id) i.id, e.id, e.organization_id
		FROM invoices i JOIN engagements e ON e.taxpayer_id = i.taxpayer_id
		WHERE e.id = ANY($1::bigint[]) AND i.source_system = $2 AND i.period = $3
		  AND i.direction = 'incoming' AND i.type = 'SATIS'
		ORDER BY i.taxpayer_id, i.payable_amount DESC, i.id`, engs, bigSource, period)
	if err != nil {
		return 0, err
	}
	type pick struct{ inv, eng, org int64 }
	picks, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (pick, error) {
		var p pick
		return p, r.Scan(&p.inv, &p.eng, &p.org)
	})
	if err != nil {
		return 0, err
	}
	invCols, err := copyColumns(ctx, tx, "invoices", "id", "uuid", "source_id", "ettn", "invoice_number", "created_at", "updated_at")
	if err != nil {
		return 0, err
	}
	itemCols, err := copyColumns(ctx, tx, "invoice_items", "id", "invoice_id")
	if err != nil {
		return 0, err
	}
	sumCols, err := copyColumns(ctx, tx, "invoice_tax_summaries", "id", "invoice_id")
	if err != nil {
		return 0, err
	}
	for _, p := range picks {
		var copyID int64
		if err := tx.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO invoices (%[1]s, source_id, ettn, invoice_number, created_at, updated_at)
			SELECT %[1]s, source_id || ':mukerrer', gen_random_uuid(),
			       left(invoice_number, 3) || to_char(issue_date, 'YYYY') || lpad((id %% 1000000000)::text, 9, '9'),
			       created_at + interval '2 days', created_at + interval '2 days'
			FROM invoices WHERE id = $1 RETURNING id`, invCols), p.inv).Scan(&copyID); err != nil {
			return 0, fmt.Errorf("invoice %d: %w", p.inv, err)
		}
		for _, t := range []struct{ table, cols string }{{"invoice_items", itemCols}, {"invoice_tax_summaries", sumCols}} {
			if _, err := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %[1]s (invoice_id, %[2]s) SELECT $2, %[2]s FROM %[1]s WHERE invoice_id = $1`,
				t.table, t.cols), p.inv, copyID); err != nil {
				return 0, fmt.Errorf("%s: %w", t.table, err)
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type, entity_id, code, severity,
				params, producer, status, created_at, updated_at)
			SELECT $3, $4, c.taxpayer_id, c.period, 'invoice', c.id, 'POSSIBLE_DUPLICATE', 'warning',
			       jsonb_build_object('candidates', jsonb_build_array(o.uuid::text), 'count', 1), 'ubl_parser', 'open',
			       c.created_at, c.created_at
			FROM invoices c, invoices o WHERE c.id = $2 AND o.id = $1`, p.inv, copyID, p.org, p.eng); err != nil {
			return 0, fmt.Errorf("warning: %w", err)
		}
	}
	return len(picks), nil
}

// copyColumns lists table's columns except skip, for INSERT … SELECT copies.
func copyColumns(ctx context.Context, tx pgx.Tx, table string, skip ...string) (string, error) {
	var cols []string
	err := tx.QueryRow(ctx, `
		SELECT array_agg(column_name::text ORDER BY ordinal_position) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 AND NOT (column_name = ANY($2::text[]))
		  AND is_generated = 'NEVER'`, table, skip).Scan(&cols)
	if err != nil {
		return "", err
	}
	if len(cols) == 0 {
		return "", fmt.Errorf("%s: no columns", table)
	}
	return strings.Join(cols, ", "), nil
}
