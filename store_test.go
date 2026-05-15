package main

import (
	"testing"
)

func TestGetSettingMissing(t *testing.T) {
	cfg = Config{SQLitePath: ":memory:"}
	var err error
	db, err = openDB(cfg.SQLitePath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	val, err := getSetting(db, "nonexistent")
	if err != nil {
		t.Errorf("getSetting for nonexistent key: %v", err)
	}
	if val != "" {
		t.Errorf("getSetting for nonexistent key = %q, want \"\"", val)
	}
}

func TestSetAndGetSetting(t *testing.T) {
	cfg = Config{SQLitePath: ":memory:"}
	var err error
	db, err = openDB(cfg.SQLitePath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := setSetting(db, "push_cutoff", "2025-01-14"); err != nil {
		t.Fatalf("setSetting: %v", err)
	}

	val, err := getSetting(db, "push_cutoff")
	if err != nil {
		t.Errorf("getSetting: %v", err)
	}
	if val != "2025-01-14" {
		t.Errorf("getSetting = %q, want %q", val, "2025-01-14")
	}

	if err := setSetting(db, "push_cutoff", "2025-06-01"); err != nil {
		t.Fatalf("setSetting upsert: %v", err)
	}

	val, err = getSetting(db, "push_cutoff")
	if err != nil {
		t.Errorf("getSetting after upsert: %v", err)
	}
	if val != "2025-06-01" {
		t.Errorf("getSetting after upsert = %q, want %q", val, "2025-06-01")
	}
}
