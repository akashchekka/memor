// rebuild.go — memor rebuild
//
// Rebuilds all indexes (trigram, bloom, tags, recency) from scratch using entries
// from the snapshot, WAL, and archive. Use after manual edits or if indexes
// become corrupted.
//
// Examples:
//   memor rebuild
package cmd

import (
	"fmt"
	"os"

	"github.com/memor-dev/memor/internal/engine"
	"github.com/memor-dev/memor/internal/index"
	"github.com/memor-dev/memor/internal/memory"
	"github.com/memor-dev/memor/internal/store"
	"github.com/spf13/cobra"
)

var rebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild all indexes from WAL + archive",
	RunE:  runRebuild,
}

func runRebuild(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	paths := store.ResolvePaths(cwd)
	if !paths.Exists() {
		return fmt.Errorf(".memor/ not found — run 'memor init' first")
	}

	// Read all entries
	snap, err := store.ReadSnapshot(paths.MemoryDB)
	if err != nil {
		return err
	}
	walEntries, err := store.ReadWAL(paths.MemoryWAL)
	if err != nil {
		return err
	}
	archiveEntries, err := store.ReadWAL(paths.Archive) // same JSONL format
	if err != nil {
		archiveEntries = nil // archive may not exist
	}

	allEntries := make([]memory.Entry, 0, len(snap.Entries)+len(walEntries)+len(archiveEntries))
	allEntries = append(allEntries, snap.Entries...)
	allEntries = append(allEntries, walEntries...)
	allEntries = append(allEntries, archiveEntries...)

	// Rebuild all indexes
	if err := paths.EnsureDirs(); err != nil {
		return err
	}

	triIdx := index.NewTrigramIndex()
	bloomIdx := index.NewBloomIndex()
	tagMap := index.NewTagMap()
	recency := index.NewRecencyRing()

	for i, e := range allEntries {
		text := e.Content
		for _, t := range e.Tags {
			text += " " + t
		}
		triIdx.Add(i, text)
		bloomIdx.Add(text)
		tagMap.Add(e.ID, e.Tags)
		recency.Touch(e.ID)
	}

	if err := bloomIdx.Save(paths.Bloom); err != nil {
		return fmt.Errorf("save bloom filter: %w", err)
	}
	if err := tagMap.Save(paths.Tags); err != nil {
		return fmt.Errorf("save tag map: %w", err)
	}
	if err := recency.Save(paths.Recency); err != nil {
		return fmt.Errorf("save recency ring: %w", err)
	}

	// Also refresh knowledge if it exists
	kb, err := engine.LoadKnowledgeDB(paths.Knowledge)
	if err == nil && len(kb.Docs) > 0 {
		refreshed, stale, err := engine.RefreshKnowledge(kb)
		if err == nil {
			if err := engine.WriteKnowledgeDB(paths.Knowledge, kb); err != nil {
				return fmt.Errorf("write knowledge.db: %w", err)
			}
			if refreshed > 0 || stale > 0 {
				fmt.Printf("Knowledge: %d refreshed, %d stale\n", refreshed, stale)
			}
		}
	}

	fmt.Printf("Rebuilt indexes for %d entries\n", len(allEntries))
	return nil
}
