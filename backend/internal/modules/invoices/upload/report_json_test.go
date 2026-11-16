package upload

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// #161: a failed file has no invoice; the report must not carry the zero
// UUID (the panel showed and linked 00000000-…).
func TestFailedItemOmitsInvoiceUUID(t *testing.T) {
	b, err := json.Marshal(ItemResult{Name: "a.xml", Status: ItemFailed, ErrorCode: CodePartyMismatch, Error: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "invoice_uuid") {
		t.Fatalf("failed item carries invoice_uuid: %s", b)
	}
	id := uuid.New()
	b, _ = json.Marshal(ItemResult{Name: "b.xml", Status: ItemAdded, InvoiceUUID: id})
	if !strings.Contains(string(b), id.String()) {
		t.Fatalf("added item lost invoice_uuid: %s", b)
	}
}
