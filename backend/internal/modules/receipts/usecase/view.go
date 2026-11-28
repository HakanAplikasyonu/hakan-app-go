package usecase

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
)

// Draft is the record a capture created.
type Draft struct {
	Type string    `json:"type"` // expense | retail_sale
	ID   int64     `json:"-"`
	UUID uuid.UUID `json:"uuid"`
}

// View is the API view of a capture.
type View struct {
	UUID        uuid.UUID                `json:"uuid"`
	Status      string                   `json:"status"`
	Kind        *string                  `json:"kind"`
	MediaType   string                   `json:"media_type"`
	Fields      map[string]extract.Value `json:"fields"`
	Model       *string                  `json:"model"`
	Draft       *Draft                   `json:"draft"`
	DuplicateOf *uuid.UUID               `json:"duplicate_of"`
	Error       *string                  `json:"error"`
	// PlateCheck: registered | not_registered | no_plate | unreadable | error
	// (capture response only).
	PlateCheck string `json:"plate_check,omitempty"`
	// Checks are the receipt checks raised on the draft (capture response
	// only; Fiş yapay zekâ kontrolleri): RECEIPT_BUYER_MISMATCH,
	// PLATE_NOT_REGISTERED, PLATE_UNREADABLE. A code the office turned off
	// is left out.
	Checks []string `json:"checks,omitempty"`
	// InReview lists the fields sent to the review queue (capture response only).
	InReview []string `json:"in_review,omitempty"`
	// Parts are the ordered photos of a document captured in several parts
	// (a long Z report); empty for a single photo.
	Parts     []PartView `json:"parts,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	// Replayed: answered from an earlier upload with the same
	// Idempotency-Key (#199); the handler then returns 200 instead of 201.
	Replayed bool `json:"-"`
}

func (s *Service) view(c db.ReceiptCapture, res *extract.Result) View {
	v := View{UUID: c.Uuid, Status: c.Status, MediaType: c.MediaType, Fields: map[string]extract.Value{},
		CreatedAt: c.CreatedAt.Time, UpdatedAt: c.UpdatedAt.Time}
	if c.Kind.Valid {
		k := c.Kind.String
		v.Kind = &k
	}
	if c.Model.Valid {
		m := c.Model.String
		v.Model = &m
	}
	if c.Error.Valid {
		e := c.Error.String
		v.Error = &e
	}
	if c.DuplicateOf.Valid {
		d := uuid.UUID(c.DuplicateOf.Bytes)
		v.DuplicateOf = &d
	}
	if res != nil {
		v.Fields = res.Fields
	} else {
		_ = json.Unmarshal(c.Fields, &v.Fields)
	}
	return v
}

// PartView is one photo of a multi-part capture.
type PartView struct {
	PartNo   int    `json:"part_no"`
	FileName string `json:"file_name"`
}

func partViews(stored []storedPart) []PartView {
	if len(stored) < 2 {
		return nil
	}
	out := make([]PartView, 0, len(stored))
	for i, p := range stored {
		out = append(out, PartView{PartNo: i + 1, FileName: p.name})
	}
	return out
}
