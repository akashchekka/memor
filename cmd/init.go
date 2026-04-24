// init.go — memor init
//
// Initializes Memor in the current project. Creates the .memor/ directory with
// config.toml, empty memory.db and memory.wal, installs a git pre-commit hook,
// adds .memor/ to .gitignore, and copies SKILL.md into AI tool skills directories.
// Imports .memor-bootstrap.jsonl if present.
//
// Flags:
//   --tools      Comma-separated tools to configure: copilot,claude,cursor,windsurf
//   --reinject   Update existing skill files to latest template
//
// Examples:
//   memor init
//   memor init --tools copilot,claude,cursor
//   memor init --reinject
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

func makeInstructions(skillPath string) string {
	return `# Memor — Project Memory

This project uses [memor](https://github.com/akashchekka/memor) for persistent AI memory.

**At the START of every conversation:**
1. Run ` + "`memor context`" + ` in the terminal to load project memory
2. Follow all instructions in ` + "`" + skillPath + "`" + `

**After EVERY response:**
1. Summarize the response in 2-3 sentences capturing the decision, reasoning, and any rejected alternatives
2. Run ` + "`memor add -s \"#tag: summary with reasoning\"`" + ` in the terminal immediately
3. Do NOT wait until the end of the conversation
4. Follow all instructions in ` + "`" + skillPath + "`" + `

**Do NOT use file-editing tools to write to ` + "`.memor/memory.wal`" + ` — always use the ` + "`memor`" + ` CLI.**
`
}

// toolInstructionFile maps each tool to its auto-discovered instruction file.
type toolInstructionFile struct {
	toolName string
	path     string // relative to project root
	content  string
}

func getToolInstructionFiles() []toolInstructionFile {
	return []toolInstructionFile{
		{"GitHub Copilot", filepath.Join(".github", "copilot-instructions.md"), makeInstructions(".github/skills/memor/SKILL.md")},
		{"Claude Code", "CLAUDE.md", makeInstructions(".claude/skills/memor/SKILL.md")},
		{"Cursor", ".cursorrules", makeInstructions(".cursor/skills/memor/SKILL.md")},
		{"Windsurf", ".windsurfrules", makeInstructions(".windsurf/skills/memor/SKILL.md")},
	}
}

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

	// Create instruction files for each configured tool
	configuredTools := make(map[string]struct{})
	for _, tc := range configs {
		configuredTools[tc.name] = struct{}{}
	}
	for _, inf := range getToolInstructionFiles() {
		if _, ok := configuredTools[inf.toolName]; !ok {
			continue
		}
		// When no --tools flag, only create Copilot instruction file by default
		if toolsFlag == "" && inf.toolName != "GitHub Copilot" {
			continue
		}
		fullPath := filepath.Join(projectRoot, inf.path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) || reinject {
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(fullPath, []byte(inf.content), 0o644); err != nil {
				return err
			}
			if reinject {
				fmt.Printf("Updated %s\n", inf.path)
			} else {
				fmt.Printf("Created %s\n", inf.path)
			}
		}
	}

	return nil
}
