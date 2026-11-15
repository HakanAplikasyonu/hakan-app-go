package sources

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"time"

	mssql "github.com/microsoft/go-mssqldb"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// Row tek bir sonuç satırıdır: kolon adı → değer. Değer türleri:
// nil, string, int64, float64, bool, time.Time, []byte. SQLQuerier
// DECIMAL / NUMERIC / MONEY değerlerini kayıpsız dizgiye,
// UNIQUEIDENTIFIER değerlerini standart GUID dizgisine çevirir.
type Row = map[string]any

// NamedQuery önceden tanımlı, adlı bir sorgudur. Text yer tutucuları
// doldurulmuş ve ValidateQuery'den geçmiş olmalıdır; Querier yine de
// yeniden denetler.
type NamedQuery struct {
	Name string
	Text string
	// Args parametre adı ('@' olmadan) → değer. Yalnızca AllowedParams.
	Args map[string]any
}

// Querier adlı sorguyu çalıştırıp en çok maxRows satır döner. Gerçek
// uygulaması SQLQuerier; testler sahte bir Querier kullanır.
type Querier interface {
	Query(ctx context.Context, q NamedQuery, maxRows int) ([]Row, error)
}

// SQLQuerier, *sql.DB üzerinde çalışan Querier'dır. Her sorgu READ
// UNCOMMITTED yalıtımlı bir işlem içinde çalışır ve işlem her zaman geri
// alınır (commit edilmez).
type SQLQuerier struct {
	db *sql.DB
}

// Open, go-mssqldb ile bağlantı havuzu açar. Bağlantı ilk sorguda kurulur.
func Open(cfg Config) (*SQLQuerier, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlserver", cfg.DSN())
	if err != nil {
		// Hata metnine DSN konmaz (parola içerir).
		return nil, rpc.Errorf(rpc.CodeInvalidParams, "bağlantı ayarları geçersiz (%s)", cfg.RedactedDSN())
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(5 * time.Minute)
	return &SQLQuerier{db: db}, nil
}

// NewSQLQuerier hazır bir *sql.DB'yi sarar.
func NewSQLQuerier(db *sql.DB) *SQLQuerier { return &SQLQuerier{db: db} }

// Close bağlantı havuzunu kapatır.
func (s *SQLQuerier) Close() error { return s.db.Close() }

// Query, Querier arayüzünü uygular.
func (s *SQLQuerier) Query(ctx context.Context, q NamedQuery, maxRows int) ([]Row, error) {
	args, err := bindArgs(q)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
	if err != nil {
		return nil, rpc.Errorf(rpc.CodeUnavailable, "SQL Server'a bağlanılamadı: %v", err)
	}
	// Salt okuma: işlem hiçbir zaman commit edilmez.
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, q.Text, args...)
	if err != nil {
		return nil, rpc.Errorf(CodeQueryFailed, "%s sorgusu çalışmadı: %v", q.Name, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.ColumnTypes()
	if err != nil {
		return nil, rpc.Errorf(CodeQueryFailed, "%s kolonları okunamadı: %v", q.Name, err)
	}
	var out []Row
	for rows.Next() {
		if maxRows > 0 && len(out) >= maxRows {
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, rpc.Errorf(CodeQueryFailed, "%s satırı okunamadı: %v", q.Name, err)
		}
		row := make(Row, len(cols))
		for i, c := range cols {
			row[c.Name()] = normalizeSQLValue(c.DatabaseTypeName(), vals[i])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, rpc.Errorf(CodeQueryFailed, "%s okunurken hata: %v", q.Name, err)
	}
	return out, nil
}

// bindArgs sorguyu yeniden denetler ve parametreleri bağlar. Metinde geçen
// her parametre verilmiş olmalı; verilen ama metinde geçmeyenler atlanır.
func bindArgs(q NamedQuery) ([]any, error) {
	used, err := ValidateQuery(q.Text)
	if err != nil {
		return nil, err
	}
	for k := range q.Args {
		if !slices.Contains(AllowedParams, k) {
			return nil, rejectf("izin verilmeyen parametre: @%s", k)
		}
	}
	args := make([]any, 0, len(used))
	for _, p := range used {
		v, ok := q.Args[p]
		if !ok {
			return nil, rejectf("%s sorgusu için @%s parametresi verilmedi", q.Name, p)
		}
		if t, ok := v.(time.Time); ok {
			// datetime olarak gönder: duvar saati korunur, saat dilimi
			// dönüşümü yapılmaz (Logo / Mikro tarih kolonları datetime).
			v = mssql.DateTime1(t)
		}
		args = append(args, sql.Named(p, v))
	}
	return args, nil
}

func normalizeSQLValue(dbType string, v any) any {
	b, ok := v.([]byte)
	if !ok {
		return v
	}
	switch strings.ToUpper(dbType) {
	case "DECIMAL", "NUMERIC", "MONEY", "SMALLMONEY":
		return string(b)
	case "UNIQUEIDENTIFIER":
		var u mssql.UniqueIdentifier
		if err := u.Scan(b); err == nil {
			return u.String()
		}
		return b
	case "BINARY", "VARBINARY", "IMAGE", "TIMESTAMP", "ROWVERSION":
		return b
	}
	return string(b)
}
