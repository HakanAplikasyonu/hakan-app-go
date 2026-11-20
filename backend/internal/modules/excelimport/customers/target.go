// Package customers writes cari cards (müşteri / tedarikçi listesi) from the
// common Excel import (#669): the accountant maps the columns, checks the
// preview and commits; each row becomes a card under the invoice rule (one
// card per VKN, or per TCKN without VKN). A card that already exists is
// counted as a duplicate and keeps its content.
package customers

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/target"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// SourceSystem names the cards written by the Excel import when the file
// names no program.
const SourceSystem = "excel_import"

// Target writes cari cards.
type Target struct{ db rls.Beginner }

// New builds the target.
func New(db rls.Beginner) Target { return Target{db: db} }

// headers are the usual column names (normalised) of a cari list.
var headers = map[string]string{
	"unvan": "name", "ticari unvan": "name", "cari unvan": "name", "cari adi": "name", "cari": "name",
	"firma": "name", "firma adi": "name", "ad soyad": "name", "adi soyadi": "name", "musteri adi": "name",
	"vkn": "tax_id", "tckn": "tax_id", "vkn tckn": "tax_id", "vergi no": "tax_id", "vergi numarasi": "tax_id",
	"vergi kimlik no": "tax_id", "tc kimlik no": "tax_id", "vkn tc": "tax_id",
	"tur": "type", "cari turu": "type", "cari tipi": "type", "tip": "type",
	"vergi dairesi": "tax_office", "vd": "tax_office",
	"adres": "address", "ilce": "district", "il": "city", "sehir": "city",
	"e posta": "email", "eposta": "email", "email": "email", "mail": "email",
	"telefon": "phone", "tel": "phone", "gsm": "phone",
}

// Heuristic maps the usual cari list headers.
func (Target) Heuristic(l parse.Layout) mapping.Mapping {
	return target.MatchHeaders(mapping.Customers, l, func(h string) string { return headers[parse.Normalize(h)] })
}

// Commit writes the rows in one transaction; a row with a wrong VKN / TCKN
// or type is reported and skipped.
func (t Target) Commit(ctx context.Context, in target.Input) (target.Result, error) {
	source := in.SourceSystem
	if source == "" || source == "unknown" {
		source = SourceSystem
	}
	res := target.Result{Errors: []target.RowError{}}
	err := rls.WithScope(ctx, t.db, rls.FromRequest(ctx), func(ctx context.Context, tx pgx.Tx) error {
		for _, r := range in.Rows {
			cp, typ, reason := party(r.Values)
			if reason != "" {
				res.Errors = append(res.Errors, target.RowError{RowNo: r.RowNo, Reason: reason})
				continue
			}
			created, err := repository.SaveCustomer(ctx, tx, in.Scope.TaxpayerID, cp, typ, source)
			if err != nil {
				return err
			}
			if created {
				res.Added++
			} else {
				res.Duplicates++
			}
		}
		return nil
	})
	if err != nil {
		return target.Result{}, err
	}
	if res.Added == 0 && res.Duplicates == 0 {
		return target.Result{}, target.Rejectf("hiçbir satır cari kartı olarak yazılamadı")
	}
	return res, nil
}

var errType = errors.New("tür müşteri, tedarikçi ya da her ikisi olmalı")

// party reads one row: the card, its type and, when the row cannot be
// written, the reason (Turkish, shown to the accountant).
func party(v map[string]string) (model.Party, string, string) {
	name := strings.TrimSpace(v["name"])
	if name == "" {
		return model.Party{}, "", "unvan boş"
	}
	id := digits(v["tax_id"])
	cp := model.Party{
		Name: name, TaxOffice: strings.TrimSpace(v["tax_office"]), Address: strings.TrimSpace(v["address"]),
		District: strings.TrimSpace(v["district"]), City: strings.TrimSpace(v["city"]),
		Email: strings.TrimSpace(v["email"]), Phone: strings.TrimSpace(v["phone"]),
	}
	switch len(id) {
	case 10:
		cp.TaxNumber, cp.LegalType = id, "corporate"
	case 11:
		cp.IdentityNumber, cp.LegalType = id, "individual"
	default:
		return model.Party{}, "", "VKN 10, TCKN 11 haneli olmalı"
	}
	typ, err := cardType(v["type"])
	if err != nil {
		return model.Party{}, "", err.Error()
	}
	return cp, typ, ""
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if r != ' ' && r != '.' && r != '-' {
			return ""
		}
	}
	return b.String()
}

func cardType(s string) (string, error) {
	n := parse.Normalize(s)
	switch {
	case n == "" || strings.Contains(n, "ikisi") || n == "both":
		return repository.CustomerTypeBoth, nil
	case strings.HasPrefix(n, "musteri") || strings.HasPrefix(n, "alici") || n == "120" || n == "customer":
		return repository.CustomerTypeCustomer, nil
	case strings.HasPrefix(n, "tedarikci") || strings.HasPrefix(n, "satici") || n == "320" || n == "supplier":
		return repository.CustomerTypeSupplier, nil
	}
	return "", errType
}
