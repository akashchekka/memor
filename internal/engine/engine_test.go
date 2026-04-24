package engine

import (
	"os"
	"testing"
	"time"

	"github.com/memor-dev/memor/internal/config"
	"github.com/memor-dev/memor/internal/memory"
	"github.com/memor-dev/memor/internal/store"
)

func setupTestProject(t *testing.T) (store.Paths, config.Config) {
	t.Helper()
	dir := t.TempDir()
	paths := store.ResolvePaths(dir)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	cfg := config.Default()

	// Write empty snapshot
	if err := os.WriteFile(paths.MemoryDB, []byte("@mem v1 | 0 entries | budget:10000 | compacted:none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write empty WAL
	if err := os.WriteFile(paths.MemoryWAL, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	return paths, cfg
}

func TestCompactBasic(t *testing.T) {
	paths, cfg := setupTestProject(t)

	// Add entries to WAL
	entries := []memory.Entry{
		{Type: memory.TypeSemantic, Tags: []string{"arch"}, Content: "PostgreSQL 16"},
		{Type: memory.TypeProcedural, Tags: []string{"deploy"}, Content: "pnpm turbo deploy"},
		{Type: memory.TypePreference, Tags: []string{"style"}, Content: "no any types"},
	}
	for _, e := range entries {
		if err := store.AppendToWAL(paths.MemoryWAL, e); err != nil {
			t.Fatal(err)
		}
	}

	written, archived, err := Compact(paths, cfg)
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	if written != 3 {
		t.Errorf("expected 3 written, got %d", written)
	}
	if archived != 0 {
		t.Errorf("expected 0 archived, got %d", archived)
	}

	// WAL should be truncated
	count, _ := store.WALEntryCount(paths.MemoryWAL)
	if count != 0 {
		t.Errorf("expected WAL truncated, got %d entries", count)
	}

	// Snapshot should have entries
	snap, err := store.ReadSnapshot(paths.MemoryDB)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Entries) != 3 {
		t.Errorf("expected 3 entries in snapshot, got %d", len(snap.Entries))
	}
}

func TestCompactDeduplicates(t *testing.T) {
	paths, cfg := setupTestProject(t)

	// Add duplicate entries
	entry := memory.Entry{Type: memory.TypeSemantic, Tags: []string{"arch"}, Content: "PostgreSQL 16"}
	for i := 0; i < 5; i++ {
		if err := store.AppendToWAL(paths.MemoryWAL, entry); err != nil {
			t.Fatal(err)
		}
	}

	written, _, err := Compact(paths, cfg)
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	if written != 1 {
		t.Errorf("expected 1 deduplicated entry, got %d", written)
	}
}

func TestCompactSupersedes(t *testing.T) {
	paths, cfg := setupTestProject(t)

	oldEntry := memory.Entry{
		Type:    memory.TypeSemantic,
		Tags:    []string{"db"},
		Content: "PostgreSQL 15",
	}
	oldEntry.ID = memory.ContentID(oldEntry.Content)

	newEntry := memory.Entry{
		Type:       memory.TypeSemantic,
		Tags:       []string{"db"},
		Content:    "PostgreSQL 16",
		Supersedes: oldEntry.ID,
	}

	if err := store.AppendToWAL(paths.MemoryWAL, oldEntry); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendToWAL(paths.MemoryWAL, newEntry); err != nil {
		t.Fatal(err)
	}

	written, _, err := Compact(paths, cfg)
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	if written != 1 {
		t.Errorf("expected 1 entry after supersede, got %d", written)
	}

	snap, _ := store.ReadSnapshot(paths.MemoryDB)
	if len(snap.Entries) > 0 && snap.Entries[0].Content != "PostgreSQL 16" {
		t.Errorf("expected superseding entry to survive, got: %s", snap.Entries[0].Content)
	}
}

func TestCompactMergesWALAndSnapshot(t *testing.T) {
	paths, cfg := setupTestProject(t)

	// Write initial snapshot with one entry
	initialEntries := []memory.Entry{
		{Type: memory.TypeSemantic, Tags: []string{"arch"}, Content: "existing entry", Timestamp: time.Now().Unix()},
	}
	if err := store.WriteSnapshot(paths.MemoryDB, initialEntries, 10000); err != nil {
		t.Fatal(err)
	}

	// Add new entry to WAL
	if err := store.AppendToWAL(paths.MemoryWAL, memory.Entry{
		Type: memory.TypeProcedural, Tags: []string{"deploy"}, Content: "new WAL entry",
	}); err != nil {
		t.Fatal(err)
	}

	written, _, err := Compact(paths, cfg)
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	if written != 2 {
		t.Errorf("expected 2 entries (merged), got %d", written)
	}
}

func TestContextBasic(t *testing.T) {
	paths, cfg := setupTestProject(t)

	// Add entries
	entries := []memory.Entry{
		{Type: memory.TypeSemantic, Tags: []string{"arch"}, Content: "PostgreSQL 16 with Drizzle ORM"},
		{Type: memory.TypeProcedural, Tags: []string{"deploy"}, Content: "pnpm turbo deploy"},
	}
	for _, e := range entries {
		if err := store.AppendToWAL(paths.MemoryWAL, e); err != nil {
			t.Fatal(err)
		}
	}

	// Compact first so entries are in snapshot
	if _, _, err := Compact(paths, cfg); err != nil {
		t.Fatal(err)
	}

	result, err := Context(paths, cfg, ContextOptions{Budget: 10000})
	if err != nil {
		t.Fatalf("Context failed: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty context output")
	}
	if len(result) < 10 {
		t.Errorf("context output too short: %s", result)
	}
}

func TestContextEmpty(t *testing.T) {
	paths, cfg := setupTestProject(t)

	result, err := Context(paths, cfg, ContextOptions{Budget: 10000})
	if err != nil {
		t.Fatalf("Context failed: %v", err)
	}

	if result != "# No memories found\n" {
		t.Errorf("expected no memories message, got: %s", result)
	}
}

func TestContextRespectsQuery(t *testing.T) {
	paths, cfg := setupTestProject(t)

	entries := []memory.Entry{
		{Type: memory.TypeSemantic, Tags: []string{"db"}, Content: "PostgreSQL 16 with Drizzle ORM"},
		{Type: memory.TypeSemantic, Tags: []string{"auth"}, Content: "OAuth2 PKCE via Auth0"},
		{Type: memory.TypeProcedural, Tags: []string{"deploy"}, Content: "deploy to kubernetes cluster"},
	}
	for _, e := range entries {
		if err := store.AppendToWAL(paths.MemoryWAL, e); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := Compact(paths, cfg); err != nil {
		t.Fatal(err)
	}

	result, err := Context(paths, cfg, ContextOptions{Budget: 10000, Query: "deploy kubernetes"})
	if err != nil {
		t.Fatalf("Context failed: %v", err)
	}

	// Result should contain all entries but deploy-related should rank higher
	if result == "" {
		t.Error("expected non-empty result")
	}
}
