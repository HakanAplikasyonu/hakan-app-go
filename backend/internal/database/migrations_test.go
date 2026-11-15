package database

import (
	"os"
	"regexp"
	"testing"
)

// TestMigrationVersionsAreUnique guards against two branches landing the same
// migration number: golang-migrate refuses to start ("duplicate migration
// file"), so every deploy would stop. Each version needs exactly one up and
// one down file.
func TestMigrationVersionsAreUnique(t *testing.T) {
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
			t.Errorf("migration %s has two %s files: %s and %s (renumber one)", v, dir, prev, name)
			continue
		}
		names[v][dir] = name
	}
	for v, dirs := range names {
		if dirs["up"] == "" || dirs["down"] == "" {
			t.Errorf("migration %s needs both up and down files", v)
		} else if dirs["up"] != dirs["down"] {
			t.Errorf("migration %s up/down names differ: %s vs %s", v, dirs["up"], dirs["down"])
		}
	}
}
