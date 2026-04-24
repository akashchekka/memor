package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/memor-dev/memor/internal/memory"
)

func TestAppendAndReadWAL(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "memory.wal")

	entries := []memory.Entry{
		{Type: memory.TypeSemantic, Tags: []string{"arch"}, Content: "PostgreSQL 16 with Drizzle"},
		{Type: memory.TypeEpisodic, Tags: []string{"bug"}, Content: "Fixed N+1 in dashboard"},
		{Type: memory.TypeProcedural, Tags: []string{"deploy"}, Content: "pnpm turbo deploy"},
	}

	for _, e := range entries {
		if err := AppendToWAL(walPath, e); err != nil {
			t.Fatalf("AppendToWAL failed: %v", err)
		}
	}

	read, err := ReadWAL(walPath)
	if err != nil {
		t.Fatalf("ReadWAL failed: %v", err)
	}

	if len(read) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(read))
	}

	if read[0].Content != "PostgreSQL 16 with Drizzle" {
		t.Errorf("unexpected content: %s", read[0].Content)
	}
	if read[0].ID == "" {
		t.Error("expected auto-generated ID")
	}
	if read[0].Timestamp == 0 {
		t.Error("expected auto-generated timestamp")
	}
}

func TestReadWALNonexistent(t *testing.T) {
	entries, err := ReadWAL("/nonexistent/memory.wal")
	if err != nil {
		t.Fatalf("expected nil error for missing WAL, got: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries, got %d", len(entries))
	}
}

func TestReadWALMalformedLines(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "memory.wal")

	content := `{"t":1713800000,"y":"s","id":"abc123","tags":["arch"],"c":"valid entry"}
this is not json
{"t":1713800100,"y":"e","id":"def456","tags":["bug"],"c":"another valid entry"}
`
	if err := os.WriteFile(walPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadWAL(walPath)
	if err != nil {
		t.Fatalf("ReadWAL failed: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries (skipping malformed), got %d", len(entries))
	}
}

func TestWALEntryCount(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "memory.wal")

	// Empty / nonexistent
	count, err := WALEntryCount(walPath)
	if err != nil {
		t.Fatalf("WALEntryCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	// Add entries
	for i := 0; i < 5; i++ {
		if err := AppendToWAL(walPath, memory.Entry{
			Type:    memory.TypeSemantic,
			Content: "entry",
			Tags:    []string{"test"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	count, err = WALEntryCount(walPath)
	if err != nil {
		t.Fatalf("WALEntryCount failed: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5, got %d", count)
	}
}

func TestWriteAndReadSnapshot(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")

	entries := []memory.Entry{
		{Type: memory.TypeSemantic, Tags: []string{"arch"}, Content: "monorepo with pnpm", Timestamp: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC).Unix()},
		{Type: memory.TypeProcedural, Tags: []string{"deploy"}, Content: "pnpm turbo deploy", Timestamp: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).Unix()},
		{Type: memory.TypePreference, Tags: []string{"typescript"}, Content: "no any types", Expires: -1},
	}

	if err := WriteSnapshot(dbPath, entries, 10000); err != nil {
		t.Fatalf("WriteSnapshot failed: %v", err)
	}

	snap, err := ReadSnapshot(dbPath)
	if err != nil {
		t.Fatalf("ReadSnapshot failed: %v", err)
	}

	if len(snap.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(snap.Entries))
	}

	// Preferences should have perm datestamp
	for _, e := range snap.Entries {
		if e.Type == memory.TypePreference && e.Expires != -1 {
			t.Error("expected preference to have perm marker")
		}
	}
}

func TestReadSnapshotNonexistent(t *testing.T) {
	snap, err := ReadSnapshot("/nonexistent/memory.db")
	if err != nil {
		t.Fatalf("expected nil error for missing snapshot, got: %v", err)
	}
	if snap.Version != "1" {
		t.Errorf("expected version 1, got %s", snap.Version)
	}
	if len(snap.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(snap.Entries))
	}
}

func TestSnapshotTokenBudget(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")

	// Create many entries that exceed a small budget
	var entries []memory.Entry
	for i := 0; i < 100; i++ {
		entries = append(entries, memory.Entry{
			Type:      memory.TypeSemantic,
			Tags:      []string{"test"},
			Content:   "this is a test entry with some content to consume tokens",
			Timestamp: time.Now().Unix(),
		})
	}

	budget := 200
	if err := WriteSnapshot(dbPath, entries, budget); err != nil {
		t.Fatalf("WriteSnapshot failed: %v", err)
	}

	snap, err := ReadSnapshot(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Should have fewer entries than 100 due to budget
	if len(snap.Entries) >= 100 {
		t.Errorf("expected budget to trim entries, got %d", len(snap.Entries))
	}
}

func TestResolvePaths(t *testing.T) {
	paths := ResolvePaths("/project")

	if paths.Root != filepath.Join("/project", ".memor") {
		t.Errorf("unexpected root: %s", paths.Root)
	}
	if paths.MemoryDB != filepath.Join("/project", ".memor", "memory.db") {
		t.Errorf("unexpected MemoryDB: %s", paths.MemoryDB)
	}
	if paths.MemoryWAL != filepath.Join("/project", ".memor", "memory.wal") {
		t.Errorf("unexpected MemoryWAL: %s", paths.MemoryWAL)
	}
	if paths.Config != filepath.Join("/project", ".memor", "config.toml") {
		t.Errorf("unexpected Config: %s", paths.Config)
	}
	if paths.Bloom != filepath.Join("/project", ".memor", "index", "bloom.bin") {
		t.Errorf("unexpected Bloom: %s", paths.Bloom)
	}
}

func TestEnsureDirs(t *testing.T) {
	dir := t.TempDir()
	paths := ResolvePaths(dir)

	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	if !paths.Exists() {
		t.Error("expected .memor/ to exist after EnsureDirs")
	}

	// Index dir should also exist
	info, err := os.Stat(paths.Index)
	if err != nil {
		t.Fatalf("index dir missing: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected index to be a directory")
	}
}
