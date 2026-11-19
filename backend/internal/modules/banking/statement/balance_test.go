package statement

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

func num(s string) pgtype.Numeric {
	d := decimal.RequireFromString(s)
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func TestBalanceCheck(t *testing.T) {
	// 10.000 + 2.500 − 1.200 = 11.300; the bank says 11.050: 250 missing out.
	c := balanceCheck(db.BankBalanceCheckRow{
		Movements: 3, InTotal: num("2500"), OutTotal: num("1200"),
		Opening: num("10000"), Closing: num("11050"),
	})
	if c.Balanced || *c.ExpectedClosing != "11300.00" || *c.Difference != "-250.00" {
		t.Fatalf("mismatch: %+v", c)
	}
	c = balanceCheck(db.BankBalanceCheckRow{
		Movements: 2, InTotal: num("100.10"), OutTotal: num("0.10"),
		Opening: num("0"), Closing: num("100"),
	})
	if !c.Balanced || *c.Difference != "0.00" {
		t.Fatalf("balanced: %+v", c)
	}
	// No balances on the statement: no verdict.
	c = balanceCheck(db.BankBalanceCheckRow{Movements: 4, WithoutBalance: 4, InTotal: num("5"), OutTotal: num("1")})
	if c.Balanced || c.Difference != nil || c.Opening != nil {
		t.Fatalf("no balance: %+v", c)
	}
}
