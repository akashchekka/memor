package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/memor-dev/memor/internal/config"
	"github.com/memor-dev/memor/internal/store"
	"github.com/spf13/cobra"
)

var initTools string
var initReinject bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize memory in the current project",
	Long:  "Creates .memor/ directory, config.toml, empty WAL and snapshot, installs git hooks, and injects instructions into AI tool configs.",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVar(&initTools, "tools", "", "Comma-separated tools to configure: copilot,claude,cursor,windsurf")
	initCmd.Flags().BoolVar(&initReinject, "reinject", false, "Update injected instructions to latest template")
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	paths := store.ResolvePaths(cwd)

	// Create directories
	if err := paths.EnsureDirs(); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}

	// Create config.toml with defaults
	if _, err := os.Stat(paths.Config); os.IsNotExist(err) {
		cfg := config.Default()
		if err := config.Save(paths.Config, cfg); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
		fmt.Println("Created", paths.Config)
	}

	// Create empty memory.db
	if _, err := os.Stat(paths.MemoryDB); os.IsNotExist(err) {
		if err := os.WriteFile(paths.MemoryDB, []byte("@mem v1 | 0 entries | budget:10000 | compacted:none\n"), 0o644); err != nil {
			return fmt.Errorf("write memory.db: %w", err)
		}
		fmt.Println("Created", paths.MemoryDB)
	}

	// Create empty memory.wal
	if _, err := os.Stat(paths.MemoryWAL); os.IsNotExist(err) {
		if err := os.WriteFile(paths.MemoryWAL, nil, 0o644); err != nil {
			return fmt.Errorf("write memory.wal: %w", err)
		}
		fmt.Println("Created", paths.MemoryWAL)
	}

	// Add .memor/ to .gitignore
	if err := ensureGitignore(cwd); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update .gitignore: %v\n", err)
	}

	// Install pre-commit hook
	if err := installPreCommitHook(cwd); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not install pre-commit hook: %v\n", err)
	}

	// Inject into AI tool configs
	if err := injectToolConfigs(cwd, initTools, initReinject); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not inject tool configs: %v\n", err)
	}

	// Import bootstrap file if exists
	bootstrapPath := filepath.Join(cwd, ".memor-bootstrap.jsonl")
	if info, err := os.Stat(bootstrapPath); err == nil && !info.IsDir() {
		entries, err := store.ReadWAL(bootstrapPath)
		if err == nil && len(entries) > 0 {
			for _, e := range entries {
				if err := store.AppendToWAL(paths.MemoryWAL, e); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not import bootstrap entry: %v\n", err)
				}
			}
			fmt.Printf("Found .memor-bootstrap.jsonl — imported %d entries\n", len(entries))
		}
	}

	fmt.Println("Memor initialized successfully.")
	return nil
}

func ensureGitignore(projectRoot string) error {
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if strings.Contains(string(content), ".memor/") {
		return nil // already present
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	entry := ".memor/\n"
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		entry = "\n" + entry
	}
	_, err = f.WriteString(entry)
	if err == nil {
		fmt.Println("Added .memor/ to .gitignore")
	}
	return err
}

func installPreCommitHook(projectRoot string) error {
	hooksDir := filepath.Join(projectRoot, ".git", "hooks")
	if _, err := os.Stat(filepath.Join(projectRoot, ".git")); os.IsNotExist(err) {
		return nil // not a git repo
	}

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}

	hookPath := filepath.Join(hooksDir, "pre-commit")

	// Check if hook already exists and has memor content
	if content, err := os.ReadFile(hookPath); err == nil {
		if strings.Contains(string(content), "memor") {
			return nil
		}
	}

	hookScript := `#!/bin/sh
# Memor pre-commit hook — auto-extract memories from staged changes
# This hook never blocks commits (always exits 0)

if command -v memor >/dev/null 2>&1; then
  memor compact --if-needed 2>/dev/null || true
fi

exit 0
`

	if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
		return err
	}
	fmt.Println("Installed .git/hooks/pre-commit (memory auto-extract)")
	return nil
}

//go:embed skill_template.md
var skillTemplate string

type toolConfig struct {
	name     string
	path     string // relative to project root
	template string
}

func getToolConfigs() []toolConfig {
	return []toolConfig{
		{
			name:     "GitHub Copilot",
			path:     filepath.Join(".github", "skills", "memor", "SKILL.md"),
			template: skillTemplate,
		},
		{
			name:     "Claude Code",
			path:     filepath.Join(".claude", "skills", "memor", "SKILL.md"),
			template: skillTemplate,
		},
		{
			name:     "Cursor",
			path:     filepath.Join(".cursor", "skills", "memor", "SKILL.md"),
			template: skillTemplate,
		},
		{
			name:     "Windsurf",
			path:     filepath.Join(".windsurf", "skills", "memor", "SKILL.md"),
			template: skillTemplate,
		},
	}
}

func injectToolConfigs(projectRoot string, toolsFlag string, reinject bool) error {
	configs := getToolConfigs()

	// If specific tools requested, filter
	if toolsFlag != "" {
		requested := make(map[string]struct{})
		for _, t := range strings.Split(toolsFlag, ",") {
			requested[strings.TrimSpace(strings.ToLower(t))] = struct{}{}
		}

		var filtered []toolConfig
		for _, tc := range configs {
			key := strings.ToLower(strings.SplitN(tc.name, " ", 2)[0])
			if _, ok := requested[key]; ok {
				filtered = append(filtered, tc)
			}
		}
		configs = filtered
	}

	for _, tc := range configs {
		fullPath := filepath.Join(projectRoot, tc.path)

		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// When no --tools flag, only create Copilot by default
			if toolsFlag == "" && tc.name != "GitHub Copilot" {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(fullPath, []byte(tc.template), 0o644); err != nil {
				return err
			}
			fmt.Printf("Created %s\n", tc.path)
		} else if reinject {
			if err := os.WriteFile(fullPath, []byte(tc.template), 0o644); err != nil {
				return err
			}
			fmt.Printf("Updated %s\n", tc.path)
		}
	}

	return nil
}
