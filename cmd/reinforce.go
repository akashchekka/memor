package cmd

import (
	"fmt"
	"os"

	"github.com/memor-dev/memor/internal/index"
	"github.com/memor-dev/memor/internal/store"
	"github.com/spf13/cobra"
)

var reinforceCmd = &cobra.Command{
	Use:   "reinforce [id]",
	Short: "Bump the relevance of a useful memory",
	Args:  cobra.ExactArgs(1),
	RunE:  runReinforce,
}

func runReinforce(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	paths := store.ResolvePaths(cwd)
	if !paths.Exists() {
		return fmt.Errorf(".memor/ not found — run 'memor init' first")
	}

	id := args[0]

	// Update recency ring
	recency := index.NewRecencyRing()
	if err := recency.Load(paths.Recency); err != nil {
		return fmt.Errorf("load recency ring: %w", err)
	}

	recency.Touch(id)

	if err := recency.Save(paths.Recency); err != nil {
		return fmt.Errorf("save recency ring: %w", err)
	}

	fmt.Printf("Reinforced memory %s (moved to front of recency ring)\n", id)
	return nil
}
