package database_test

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

func TestQuerierInterfaceCompiles(t *testing.T) {
	t.Parallel()

	var _ db.Querier = (*db.Queries)(nil)
}
