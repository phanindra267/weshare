package database

import "testing"

func TestLoadMigrationsParsesEmbeddedFiles(t *testing.T) {
	migs, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migs) == 0 {
		t.Fatal("expected at least one embedded migration")
	}

	// Migrations must be sorted ascending by version.
	for i := 1; i < len(migs); i++ {
		if migs[i-1].version >= migs[i].version {
			t.Fatalf("migrations not sorted: %d then %d", migs[i-1].version, migs[i].version)
		}
	}

	first := migs[0]
	if first.version != 1 {
		t.Errorf("first version = %d, want 1", first.version)
	}
	if first.name != "init" {
		t.Errorf("first name = %q, want init", first.name)
	}
	if first.upSQL == "" {
		t.Error("first migration has empty up SQL")
	}
	if !first.hasDown || first.downSQL == "" {
		t.Error("first migration is missing its down SQL")
	}
}
