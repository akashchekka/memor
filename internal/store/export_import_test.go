package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/memor-dev/memor/internal/memory"
)

func TestExportImportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "memory.wal")
	exportPath := filepath.Join(dir, "export.jsonl")

	// Create test entries
	entries := []memory.Entry{
		{Timestamp: time.Now().Unix(), Type: memory.TypeSemantic, Content: "PostgreSQL 16 with Drizzle", Tags: []string{"arch", "db"}, ID: memory.ContentID("PostgreSQL 16 with Drizzle")},
		{Timestamp: time.Now().Unix(), Type: memory.TypeEpisodic, Content: "Fixed N+1 in dashboard", Tags: []string{"perf"}, ID: memory.ContentID("Fixed N+1 in dashboard")},
		{Timestamp: time.Now().Unix(), Type: memory.TypeProcedural, Content: "pnpm turbo deploy", Tags: []string{"deploy"}, ID: memory.ContentID("pnpm turbo deploy")},
		{Timestamp: time.Now().Unix(), Type: memory.TypePreference, Content: "No any, use unknown", Tags: []string{"typescript"}, ID: memory.ContentID("No any, use unknown")},
		{Timestamp: time.Now().Unix(), Type: memory.TypeCode, Content: "src/lib/auth.ts", Tags: []string{"auth"}, ID: memory.ContentID("src/lib/auth.ts"),
			Meta: &memory.CodeMeta{FilePath: "src/lib/auth.ts", LOC: 187, Hash: "a3f9c2", Exports: []string{"refreshToken()", "validateSession()"}, Summary: "Auth middleware"}},
	}

	// Write entries to WAL
	for _, e := range entries {
		if err := AppendToWAL(walPath, e); err != nil {
			t.Fatalf("AppendToWAL failed: %v", err)
		}
	}

	// Read WAL back
	readEntries, err := ReadWAL(walPath)
	if err != nil {
		t.Fatalf("ReadWAL failed: %v", err)
	}
	if len(readEntries) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(readEntries))
	}

	// Export: write as JSONL
	f, err := os.Create(exportPath)
	if err != nil {
		t.Fatalf("create export file: %v", err)
	}
	encoder := json.NewEncoder(f)
	for _, e := range readEntries {
		if err := encoder.Encode(e); err != nil {
			t.Fatalf("encode entry: %v", err)
		}
	}
	f.Close()

	// Import: read JSONL back
	importData, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}

	var imported []memory.Entry
	for _, line := range strings.Split(strings.TrimSpace(string(importData)), "\n") {
		if line == "" {
			continue
		}
		var e memory.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal line: %v", err)
		}
		imported = append(imported, e)
	}

	if len(imported) != len(entries) {
		t.Fatalf("round-trip: expected %d entries, got %d", len(entries), len(imported))
	}

	// Verify each entry preserved
	for i, e := range imported {
		if e.Type != entries[i].Type {
			t.Errorf("entry %d: type mismatch: got %s, want %s", i, e.Type, entries[i].Type)
		}
		if e.Content != entries[i].Content {
			t.Errorf("entry %d: content mismatch: got %q, want %q", i, e.Content, entries[i].Content)
		}
		if e.ID != entries[i].ID {
			t.Errorf("entry %d: ID mismatch: got %s, want %s", i, e.ID, entries[i].ID)
		}
	}

	// Verify @c entry preserved Meta
	lastImported := imported[len(imported)-1]
	if lastImported.Meta == nil {
		t.Fatal("@c entry lost Meta during round-trip")
	}
	if lastImported.Meta.FilePath != "src/lib/auth.ts" {
		t.Errorf("Meta.FilePath: got %q, want %q", lastImported.Meta.FilePath, "src/lib/auth.ts")
	}
	if lastImported.Meta.LOC != 187 {
		t.Errorf("Meta.LOC: got %d, want 187", lastImported.Meta.LOC)
	}
	if lastImported.Meta.Hash != "a3f9c2" {
		t.Errorf("Meta.Hash: got %q, want %q", lastImported.Meta.Hash, "a3f9c2")
	}
	if len(lastImported.Meta.Exports) != 2 {
		t.Errorf("Meta.Exports: got %d, want 2", len(lastImported.Meta.Exports))
	}
	if lastImported.Meta.Summary != "Auth middleware" {
		t.Errorf("Meta.Summary: got %q, want %q", lastImported.Meta.Summary, "Auth middleware")
	}
}

func TestExportFilterByType(t *testing.T) {
	entries := []memory.Entry{
		{Type: memory.TypeSemantic, Content: "fact 1", ID: "aaa"},
		{Type: memory.TypeEpisodic, Content: "event 1", ID: "bbb"},
		{Type: memory.TypeSemantic, Content: "fact 2", ID: "ccc"},
		{Type: memory.TypeProcedural, Content: "cmd 1", ID: "ddd"},
	}

	typeFilter := map[memory.Type]struct{}{
		memory.TypeSemantic: {},
	}

	var filtered []memory.Entry
	for _, e := range entries {
		if _, ok := typeFilter[e.Type]; ok {
			filtered = append(filtered, e)
		}
	}

	if len(filtered) != 2 {
		t.Errorf("expected 2 semantic entries, got %d", len(filtered))
	}
	for _, e := range filtered {
		if e.Type != memory.TypeSemantic {
			t.Errorf("expected semantic type, got %s", e.Type)
		}
	}
}

func TestExportFilterByTags(t *testing.T) {
	entries := []memory.Entry{
		{Type: memory.TypeSemantic, Content: "auth decision", Tags: []string{"auth", "api"}, ID: "aaa"},
		{Type: memory.TypeSemantic, Content: "db decision", Tags: []string{"db"}, ID: "bbb"},
		{Type: memory.TypeSemantic, Content: "auth and db", Tags: []string{"auth", "db"}, ID: "ccc"},
	}

	tagFilter := map[string]struct{}{
		"auth": {},
	}

	var filtered []memory.Entry
	for _, e := range entries {
		for _, tag := range e.Tags {
			if _, ok := tagFilter[tag]; ok {
				filtered = append(filtered, e)
				break
			}
		}
	}

	if len(filtered) != 2 {
		t.Errorf("expected 2 entries with #auth, got %d", len(filtered))
	}
}

func TestExportFilterBySince(t *testing.T) {
	now := time.Now()
	entries := []memory.Entry{
		{Timestamp: now.AddDate(0, 0, -10).Unix(), Type: memory.TypeSemantic, Content: "old", ID: "aaa"},
		{Timestamp: now.AddDate(0, 0, -3).Unix(), Type: memory.TypeSemantic, Content: "recent", ID: "bbb"},
		{Timestamp: now.Unix(), Type: memory.TypeSemantic, Content: "today", ID: "ccc"},
	}

	sinceTime := now.AddDate(0, 0, -5)

	var filtered []memory.Entry
	for _, e := range entries {
		if e.Timestamp >= sinceTime.Unix() {
			filtered = append(filtered, e)
		}
	}

	if len(filtered) != 2 {
		t.Errorf("expected 2 recent entries, got %d", len(filtered))
	}
}

func TestImportSkipDuplicates(t *testing.T) {
	existingIDs := map[string]struct{}{
		"aaa": {},
		"bbb": {},
	}

	incoming := []memory.Entry{
		{ID: "aaa", Content: "existing 1"},
		{ID: "ccc", Content: "new entry"},
		{ID: "bbb", Content: "existing 2"},
		{ID: "ddd", Content: "another new"},
	}

	var toImport []memory.Entry
	skipped := 0
	for _, e := range incoming {
		if _, exists := existingIDs[e.ID]; exists {
			skipped++
			continue
		}
		toImport = append(toImport, e)
	}

	if len(toImport) != 2 {
		t.Errorf("expected 2 new entries, got %d", len(toImport))
	}
	if skipped != 2 {
		t.Errorf("expected 2 skipped, got %d", skipped)
	}
	if toImport[0].ID != "ccc" || toImport[1].ID != "ddd" {
		t.Errorf("wrong entries imported: %v", toImport)
	}
}

func TestImportAddTag(t *testing.T) {
	entries := []memory.Entry{
		{ID: "aaa", Content: "fact 1", Tags: []string{"arch"}},
		{ID: "bbb", Content: "fact 2", Tags: nil},
	}

	importTag := "imported"
	for i := range entries {
		entries[i].Tags = append(entries[i].Tags, importTag)
	}

	if len(entries[0].Tags) != 2 || entries[0].Tags[1] != "imported" {
		t.Errorf("expected [arch, imported], got %v", entries[0].Tags)
	}
	if len(entries[1].Tags) != 1 || entries[1].Tags[0] != "imported" {
		t.Errorf("expected [imported], got %v", entries[1].Tags)
	}
}
