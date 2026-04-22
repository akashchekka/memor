package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/memor-dev/memor/internal/store"
	"github.com/spf13/cobra"
)

var purgeAll bool

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Remove all memor files from the current project",
	Long:  "Deletes .memor/ directory and optionally removes injected instructions from AI tool config files.",
	RunE:  runPurge,
}

func init() {
	purgeCmd.Flags().BoolVar(&purgeAll, "all", false, "Also remove injected instructions from AI tool config files (.github/copilot-instructions.md, CLAUDE.md, .cursorrules, .windsurfrules)")
}

func runPurge(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	paths := store.ResolvePaths(cwd)
	if !paths.Exists() {
		fmt.Println("Nothing to purge — .memor/ does not exist.")
		return nil
	}

	// Remove .memor/ directory
	if err := os.RemoveAll(paths.Root); err != nil {
		return fmt.Errorf("remove .memor/: %w", err)
	}
	fmt.Println("Removed .memor/")

	if purgeAll {
		for _, tc := range getToolConfigs() {
			fullPath := filepath.Join(cwd, tc.path)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				continue
			}
			if err := os.Remove(fullPath); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", tc.path, err)
				continue
			}
			fmt.Printf("Removed %s\n", tc.path)
		}

		// Clean up empty .github/ directory if we created it
		githubDir := filepath.Join(cwd, ".github")
		entries, err := os.ReadDir(githubDir)
		if err == nil && len(entries) == 0 {
			os.Remove(githubDir)
		}
	}

	fmt.Println("Purge complete.")
	return nil
}
