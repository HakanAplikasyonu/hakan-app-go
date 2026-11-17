package mapping

import "testing"

func TestEveryDataTypeHasACatalog(t *testing.T) {
	for _, dt := range SupportedDataTypes {
		c, ok := CatalogFor(dt)
		if !ok || c.Label == "" || len(c.Fields) == 0 || len(c.Required) == 0 {
			t.Fatalf("%s: catalog %+v", dt, c)
		}
		for _, g := range c.Required {
			for _, f := range g {
				if !ValidField(dt, f) {
					t.Fatalf("%s: required field %s is not a field", dt, f)
				}
			}
		}
	}
	if !ValidField(Expenses, "total") || ValidField(Expenses, "invoice_number") || !ValidField(Payroll, "net") {
		t.Fatal("field validation by data type")
	}
	if c, _ := CatalogFor(BankTransactions); c.Kind("debit") != KindMoney || c.Kind("date") != KindDate || c.Kind("x") != KindText {
		t.Fatal("kinds")
	}
}

func TestMissingRequiredPerDataType(t *testing.T) {
	mp := Mapping{DataType: BankTransactions, Columns: []ColumnMap{
		{Column: "A", Field: "date"}, {Column: "B", Field: "description"}, {Column: "C", Field: "debit"},
	}}
	if m := MissingRequired(BankTransactions, mp); len(m) != 0 {
		t.Fatalf("bank with debit only: %v", m)
	}
	if err := mp.Validate(); err != nil {
		t.Fatal(err)
	}
	mp.DataType = Expenses
	if m := MissingRequired(Expenses, mp); len(m) != 1 || m[0][0] != "total" {
		t.Fatalf("expenses: %v", m)
	}
	if err := mp.Validate(); err == nil {
		t.Fatal("bank field accepted for expenses")
	}
}
