package reviewlink

import (
	"context"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receipts "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	review "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
)

type capSubmitter struct{ in review.Input }

func (c *capSubmitter) Submit(_ context.Context, _ review.Subject, in review.Input) (review.Suggestion, error) {
	c.in = in
	return review.Suggestion{}, nil
}

// Mükellef düzeltmesi girdi kökenli öneri olur, model kökenli değil (#199).
func TestTaxpayerProposalIsInputOrigin(t *testing.T) {
	sub := &capSubmitter{}
	err := NewReviewer(sub).SubmitField(context.Background(), receipts.FieldReview{
		OrganizationID: 1, EngagementID: 2, TaxpayerID: 3, TargetType: "expense", TargetID: 9, Period: "2026-10",
		Field: extract.FieldTotal, Value: extract.Value{Raw: "1.250,00", Value: "1250.00", Confidence: 1, Valid: true},
		ProposedBy: 42, Note: "fişte 1.250 yazıyor",
	})
	if err != nil {
		t.Fatal(err)
	}
	in := sub.in
	if in.Provenance.Origin != review.OriginInput || in.Provenance.Model != "" {
		t.Fatalf("provenance = %+v", in.Provenance)
	}
	if in.Rationale != "Mükellef düzeltmesi: fişte 1.250 yazıyor" || string(in.Value) != `"1250.00"` || !in.Confidence.Equal(in.Confidence.Round(4)) {
		t.Fatalf("input = %+v", in)
	}
	if in.Provenance.Inputs["proposed_by"] != "taxpayer" || in.Provenance.Inputs["user_id"] != int64(42) {
		t.Fatalf("inputs = %v", in.Provenance.Inputs)
	}
}
