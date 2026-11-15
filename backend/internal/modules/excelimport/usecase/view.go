package usecase

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/build"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

// Summary is the preview (04 §5) plus the commit / rollback outcome.
type Summary struct {
	build.Summary
	// Existing rows are already in the database (same source key).
	Existing   int `json:"existing"`
	Added      int `json:"added"`
	Duplicates int `json:"duplicates"`
	RolledBack int `json:"rolled_back"`
}

// View is the API view of an import.
type View struct {
	UUID         uuid.UUID        `json:"uuid"`
	FileName     string           `json:"file_name"`
	Format       string           `json:"format"`
	Status       string           `json:"status"`
	DataType     *string          `json:"data_type"`
	SourceSystem *string          `json:"source_system"`
	Period       *string          `json:"period"`
	Sheet        *string          `json:"sheet"`
	Layout       *parse.Layout    `json:"layout"`
	Mapping      *mapping.Mapping `json:"mapping"`
	FromTemplate bool             `json:"from_template"`
	AIUsed       bool             `json:"ai_used"`
	Summary      *Summary         `json:"summary"`
	Error        *string          `json:"error"`
	CommittedAt  *time.Time       `json:"committed_at"`
	RolledBackAt *time.Time       `json:"rolled_back_at"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

func (s *Service) view(r db.ExcelImport) View {
	v := View{
		UUID: r.Uuid, FileName: r.FileName, Format: r.Format, Status: r.Status,
		DataType: str(r.DataType.String), SourceSystem: str(r.SourceSystem.String), Period: str(r.Period.String),
		Sheet: str(r.SheetName.String), FromTemplate: r.TemplateID.Valid, AIUsed: r.AiUsed, Error: str(r.Error.String),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	var l parse.Layout
	if json.Unmarshal(r.Layout, &l) == nil && l.HeaderRow > 0 {
		v.Layout = &l
	}
	var m mapping.Mapping
	if json.Unmarshal(r.Mapping, &m) == nil && len(m.Columns) > 0 {
		v.Mapping = &m
	}
	var sm Summary
	if json.Unmarshal(r.Summary, &sm) == nil && sm.Rows > 0 {
		v.Summary = &sm
	}
	if r.CommittedAt.Valid {
		t := r.CommittedAt.Time
		v.CommittedAt = &t
	}
	if r.RolledBackAt.Valid {
		t := r.RolledBackAt.Time
		v.RolledBackAt = &t
	}
	return v
}

func str(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
