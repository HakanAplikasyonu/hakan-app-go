// Package fieldcrypt wires field-level encryption (#168) from configuration
// and moves stored values between plaintext and sealed form.
//
// The cipher itself lives in internal/platform/secrets (field.go). Write
// paths call secrets.SealField and read paths secrets.OpenField; this package
// builds the process-wide cipher from config and runs the data migration
// (encrypt existing rows, decrypt them back, re-wrap after a key rotation).
// See backend/docs/alan-sifreleme.md.
package fieldcrypt

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
)

// FromConfig builds the field cipher. It refuses field keys that equal
// APP_ENCRYPTION_KEY or a vault key. A nil cipher (no field key at all) means
// encryption is off; the config guard refuses that outside development.
func FromConfig(c config.EncryptionConfig) (*secrets.FieldCipher, error) {
	var forbid []*secrets.Keyring
	if vault, err := secrets.ParseKeyring(c.SecretsKeyring, c.SecretsActiveKeyID, c.Key); err == nil {
		forbid = append(forbid, vault)
	}
	kr, err := secrets.ParseFieldKeyring(c.FieldKeyring, c.FieldActiveKeyID, c.FieldKey, forbid...)
	if err != nil {
		return nil, err
	}
	return secrets.NewFieldCipher(kr), nil
}

// Install builds the cipher from config and installs it process-wide.
func Install(c config.EncryptionConfig) (*secrets.FieldCipher, error) {
	fc, err := FromConfig(c)
	if err != nil {
		return nil, err
	}
	secrets.InstallFieldCipher(fc)
	return fc, nil
}

// Column is one encrypted JSONB column.
type Column struct {
	Table   string
	Column  string
	Purpose string
}

// Name is "table.column".
func (c Column) Name() string { return c.Table + "." + c.Column }

// Columns lists every encrypted column. Table and column names come only from
// this list, never from input, so the dynamic SQL below is safe.
var Columns = []Column{
	{"bank_transactions", "raw_payload", secrets.PurposeBankRawPayload},
	{"bank_transaction_sources", "raw_payload", secrets.PurposeBankRawPayload},
	{"invoices", "raw_payload", secrets.PurposeInvoiceRawPayload},
	{"invoice_sources", "raw_payload", secrets.PurposeInvoiceRawPayload},
	{"customers", "raw_payload", secrets.PurposeCustomerRawPayload},
	{"customer_source_links", "raw_payload", secrets.PurposeCustomerRawPayload},
	{"expenses", "raw_payload", secrets.PurposeExpenseRawPayload},
	{"retail_sales", "raw_payload", secrets.PurposeRetailSaleRawPayload},
	{"entegrator_documents", "header_raw", secrets.PurposeEntegratorHeader},
	{"source_financial_transactions", "raw_payload", secrets.PurposeFinancialTxRawPayload},
}

// Mode is what Run does to each value.
type Mode string

const (
	// Encrypt seals plaintext values (the forward migration).
	Encrypt Mode = "encrypt"
	// Decrypt turns sealed values back into plaintext (the reverse migration).
	Decrypt Mode = "decrypt"
	// Rotate re-wraps data keys of values not on the active KEK.
	Rotate Mode = "rotate"
)

// ColumnReport counts what Run did on one column.
type ColumnReport struct {
	Column  string
	Changed int
	// Skipped: the row changed between read and write; a rerun picks it up.
	Skipped int
	// Failed: the value could not be opened (unknown KEK, tampered).
	Failed int
}

// Report is the outcome of Run.
type Report struct {
	Mode    Mode
	Active  string
	Columns []ColumnReport
}

// Failed is the total of failed values.
func (r Report) Failed() int {
	n := 0
	for _, c := range r.Columns {
		n += c.Failed
	}
	return n
}

// Run applies mode to every row of the selected columns (all when only is
// empty), batch rows per transaction. It is idempotent and resumable: each
// mode only selects the rows it still has to change. The connection must
// bypass row-level security (the migration/owner role does).
func Run(ctx context.Context, pool *pgxpool.Pool, fc *secrets.FieldCipher, mode Mode, batch int, only []string) (Report, error) {
	if fc == nil {
		return Report{}, errors.New("fieldcrypt: no field key configured (FIELD_ENCRYPTION_KEY)")
	}
	if batch <= 0 {
		batch = 500
	}
	rep := Report{Mode: mode, Active: fc.ActiveID()}
	for _, col := range Columns {
		if len(only) > 0 && !slices.Contains(only, col.Name()) && !slices.Contains(only, col.Table) {
			continue
		}
		cr, err := runColumn(ctx, pool, fc, mode, batch, col)
		rep.Columns = append(rep.Columns, cr)
		if err != nil {
			return rep, fmt.Errorf("fieldcrypt: %s: %w", col.Name(), err)
		}
	}
	return rep, nil
}

func runColumn(ctx context.Context, pool *pgxpool.Pool, fc *secrets.FieldCipher, mode Mode, batch int, col Column) (ColumnReport, error) {
	cr := ColumnReport{Column: col.Name()}
	var filter string
	switch mode {
	case Encrypt:
		filter = fmt.Sprintf("NOT (%[1]s ? '$enc') AND %[1]s <> '{}'::jsonb", col.Column)
	case Decrypt:
		filter = fmt.Sprintf("%s ? '$enc'", col.Column)
	case Rotate:
		filter = fmt.Sprintf("%[1]s ? '$enc' AND %[1]s->>'kek' IS DISTINCT FROM $3", col.Column)
	default:
		return cr, fmt.Errorf("unknown mode %q", mode)
	}
	sel := fmt.Sprintf(`SELECT id, %s::text FROM %s WHERE id > $1 AND %s ORDER BY id LIMIT $2`, col.Column, col.Table, filter)
	upd := fmt.Sprintf(`UPDATE %[1]s SET %[2]s = $2::jsonb WHERE id = $1 AND %[2]s = $3::jsonb`, col.Table, col.Column)

	var after int64
	for {
		args := []any{after, batch}
		if mode == Rotate {
			args = append(args, fc.ActiveID())
		}
		type row struct {
			id  int64
			val string
		}
		rows, err := pool.Query(ctx, sel, args...)
		if err != nil {
			return cr, err
		}
		list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
			var x row
			return x, r.Scan(&x.id, &x.val)
		})
		if err != nil {
			return cr, err
		}
		if len(list) == 0 {
			return cr, nil
		}
		err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			for _, r := range list {
				next, changed, err := transform(fc, mode, col.Purpose, []byte(r.val))
				if err != nil {
					cr.Failed++
					continue
				}
				if !changed {
					continue
				}
				tag, err := tx.Exec(ctx, upd, r.id, string(next), r.val)
				if err != nil {
					return err
				}
				if tag.RowsAffected() == 1 {
					cr.Changed++
				} else {
					cr.Skipped++
				}
			}
			return nil
		})
		if err != nil {
			return cr, err
		}
		after = list[len(list)-1].id
	}
}

func transform(fc *secrets.FieldCipher, mode Mode, purpose string, val []byte) ([]byte, bool, error) {
	switch mode {
	case Encrypt:
		if secrets.IsSealedField(val) {
			return val, false, nil
		}
		out, err := fc.Seal(purpose, val)
		return out, err == nil, err
	case Decrypt:
		if !secrets.IsSealedField(val) {
			return val, false, nil
		}
		out, err := fc.Open(purpose, val)
		return out, err == nil, err
	default:
		return fc.Rewrap(purpose, val)
	}
}

// KEKCount is the number of values of one column on one KEK ("" = plaintext).
type KEKCount struct {
	Column string
	KEK    string
	Count  int64
}

// Status counts values per column and KEK id, so an operator can see what is
// still plaintext and which keys are still in use before dropping one.
func Status(ctx context.Context, pool *pgxpool.Pool) ([]KEKCount, error) {
	var out []KEKCount
	for _, col := range Columns {
		q := fmt.Sprintf(`SELECT COALESCE(CASE WHEN %[1]s ? '$enc' THEN %[1]s->>'kek' END, ''), count(*)
			FROM %[2]s WHERE %[1]s <> '{}'::jsonb GROUP BY 1 ORDER BY 1`, col.Column, col.Table)
		rows, err := pool.Query(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("fieldcrypt: %s: %w", col.Name(), err)
		}
		counts, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (KEKCount, error) {
			k := KEKCount{Column: col.Name()}
			return k, r.Scan(&k.KEK, &k.Count)
		})
		if err != nil {
			return nil, fmt.Errorf("fieldcrypt: %s: %w", col.Name(), err)
		}
		out = append(out, counts...)
	}
	return out, nil
}
