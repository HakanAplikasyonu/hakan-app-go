package database

import (
	"os"
	"regexp"
	"testing"
)

// TestMigrationVersionsUnique: golang-migrate refuses two files with the
// same version, and parallel PRs can each add one (it happened with 000042).
// Every version must have exactly one up and one down file.
func TestMigrationVersionsUnique(t *testing.T) {
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)
	names := map[string]map[string]string{} // version -> direction -> name
	for _, e := range entries {
		m := re.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, name, dir := m[1], m[2], m[3]
		if names[v] == nil {
			names[v] = map[string]string{}
		}
		if prev, ok := names[v][dir]; ok {
			t.Errorf("migration version %s used twice: %s and %s (renumber the newer one)", v, prev, name)
			continue
		}
		names[v][dir] = name
	}
	for v, d := range names {
		if d["up"] == "" || d["down"] == "" {
			t.Errorf("migration %s needs both up and down files", v)
		} else if d["up"] != d["down"] {
			t.Errorf("migration %s: up (%s) and down (%s) names differ", v, d["up"], d["down"])
		}
	}
}
