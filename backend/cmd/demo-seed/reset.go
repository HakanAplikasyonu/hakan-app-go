package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
)

// dropDemo deletes the demo office and the data of its demo taxpayers (#148).
//
// Scope, so a reset never touches real tenant data:
//   - the office is found by demo.OrgSlug only;
//   - a taxpayer's data is deleted only when the taxpayer is demo-only: its
//     tax id is one of demo.Taxpayers and no other office has (or had) an
//     engagement with it. Otherwise the identity and its data are kept and
//     only the demo office's own rows go (ON DELETE CASCADE from the office).
//
// Taxpayer data (invoices, customers, bank lines, sources, …) references the
// taxpayer with RESTRICT, so it is deleted child-first by walking the foreign
// keys in pg_catalog (deleteGraph); new tables need no change here.
// Users are kept and reused.
func dropDemo(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var orgID int64
	err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE slug = $1`, demo.OrgSlug).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	demoIDs := make([]string, 0, len(demo.Taxpayers))
	for _, tp := range demo.Taxpayers {
		demoIDs = append(demoIDs, tp.TaxID)
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT t.id FROM taxpayers t
		JOIN engagements e ON e.taxpayer_id = t.id AND e.organization_id = $1
		WHERE t.tax_id = ANY($2::text[])
		  AND NOT EXISTS (SELECT 1 FROM engagements o WHERE o.taxpayer_id = t.id AND o.organization_id <> $1)
		ORDER BY t.id`, orgID, demoIDs)
	if err != nil {
		return err
	}
	owned, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return err
	}

	if len(owned) > 0 {
		fks, err := loadForeignKeys(ctx, tx)
		if err != nil {
			return err
		}
		ids := make([]string, len(owned))
		for i, id := range owned {
			ids[i] = strconv.FormatInt(id, 10)
		}
		root := "id = ANY('{" + strings.Join(ids, ",") + "}'::bigint[])"
		if err := deleteGraph(ctx, tx, fks, "taxpayers", root, map[string]bool{"taxpayers": true}); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID); err != nil {
		return fmt.Errorf("delete demo office: %w", err)
	}
	return tx.Commit(ctx)
}

// foreignKey is one FK edge child(cols) -> parent(refCols).
type foreignKey struct {
	child, parent string
	cols, refCols []string
	onDelete      byte // pg_constraint.confdeltype
}

func loadForeignKeys(ctx context.Context, tx pgx.Tx) (map[string][]foreignKey, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.conrelid::regclass::text, c.confrelid::regclass::text, c.confdeltype::text,
		       ARRAY(SELECT quote_ident(a.attname) FROM unnest(c.conkey) WITH ORDINALITY k(n, i)
		             JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.n ORDER BY k.i),
		       ARRAY(SELECT quote_ident(a.attname) FROM unnest(c.confkey) WITH ORDINALITY k(n, i)
		             JOIN pg_attribute a ON a.attrelid = c.confrelid AND a.attnum = k.n ORDER BY k.i)
		FROM pg_constraint c
		JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE c.contype = 'f' AND n.nspname = current_schema()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byParent := map[string][]foreignKey{}
	for rows.Next() {
		var fk foreignKey
		var del string
		if err := rows.Scan(&fk.child, &fk.parent, &del, &fk.cols, &fk.refCols); err != nil {
			return nil, err
		}
		fk.onDelete = del[0]
		byParent[fk.parent] = append(byParent[fk.parent], fk)
	}
	return byParent, rows.Err()
}

// deleteGraph deletes the rows of table matching where, after deleting every
// row that references them (recursively). SET NULL / SET DEFAULT edges are
// left to Postgres; a table already on the path (cycle or self reference) is
// not entered again.
func deleteGraph(ctx context.Context, tx pgx.Tx, fks map[string][]foreignKey, table, where string, path map[string]bool) error {
	for _, fk := range fks[table] {
		if fk.onDelete == 'n' || fk.onDelete == 'd' || path[fk.child] {
			continue
		}
		childWhere := fmt.Sprintf("(%s) IN (SELECT %s FROM %s WHERE %s)",
			strings.Join(fk.cols, ", "), strings.Join(fk.refCols, ", "), table, where)
		path[fk.child] = true
		err := deleteGraph(ctx, tx, fks, fk.child, childWhere, path)
		delete(path, fk.child)
		if err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE "+where); err != nil {
		return fmt.Errorf("delete %s: %w", table, err)
	}
	return nil
}
