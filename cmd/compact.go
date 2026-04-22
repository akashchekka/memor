package cmd

import (
	"fmt"
	"os"

	"github.com/memor-dev/memor/internal/config"
	"github.com/memor-dev/memor/internal/engine"
	"github.com/memor-dev/memor/internal/store"
	"github.com/spf13/cobra"
)

var compactIfNeeded bool

var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Run compaction — merge WAL into memory.db",
	Long:  "Parses the WAL, deduplicates, scores, and writes a fresh token-budgeted snapshot to memory.db.",
	RunE:  runCompact,
}

func init() {
	compactCmd.Flags().BoolVar(&compactIfNeeded, "if-needed", false, "Only compact if WAL exceeds threshold")
}

func runCompact(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	paths := store.ResolvePaths(cwd)
	if !paths.Exists() {
		return fmt.Errorf(".memor/ not found — run 'memor init' first")
	}

	cfg, err := config.Load(paths.Config)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if compactIfNeeded {
		count, err := store.WALEntryCount(paths.MemoryWAL)
		if err != nil {
			return err
		}
		if count < cfg.Memory.WALMaxEntries {
			return nil // not needed yet
		}
	}

	written, archived, err := engine.Compact(paths, cfg)
	if err != nil {
		return err
	}

	fmt.Printf("Compaction complete: %d entries in snapshot, %d archived\n", written, archived)
	return nil
}
