// purge.go — memor purge
//
// Deletes the entire .memor/ directory. With --all, also removes skill files
// from AI tool directories and cleans up empty parent directories.
//
// Flags:
//   --all   Also remove skill files from AI tool directories
//
// Examples:
//   memor purge
//   memor purge --all
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
			// Remove the skills/memor/ directory for this tool
			skillDir := filepath.Dir(filepath.Join(cwd, tc.path))
			if _, err := os.Stat(skillDir); os.IsNotExist(err) {
				continue
			}
			if err := os.RemoveAll(skillDir); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", filepath.Dir(tc.path), err)
				continue
			}
			fmt.Printf("Removed %s\n", filepath.Dir(tc.path))

			// Clean up empty parent directories (skills/, .github/, etc.)
			dir := filepath.Dir(skillDir)
			for dir != cwd {
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) > 0 {
					break
				}
				os.Remove(dir)
				dir = filepath.Dir(dir)
			}
		}

		// Remove instruction files (copilot-instructions.md, CLAUDE.md, .cursorrules, .windsurfrules)
		for _, inf := range getToolInstructionFiles() {
			fullPath := filepath.Join(cwd, inf.path)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				continue
			}
			if err := os.Remove(fullPath); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", inf.path, err)
				continue
			}
			fmt.Printf("Removed %s\n", inf.path)
		}
	}

	fmt.Println("Purge complete.")
	return nil
}
