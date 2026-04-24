package config

import (
	"os"
	"path/filepath"

	"github.com/memor-dev/memor/internal/constants"
	toml "github.com/pelletier/go-toml/v2"
)

// Config represents the full config.toml structure.
type Config struct {
	Memory     MemoryConfig     `toml:"memory"`
	Compaction CompactionConfig `toml:"compaction"`
	Knowledge  KnowledgeConfig  `toml:"knowledge"`
	Hooks      HooksConfig      `toml:"hooks"`
}

// MemoryConfig holds memory budget and WAL settings.
type MemoryConfig struct {
	SchemaVersion    string `toml:"schema_version"`
	TokenBudget      int    `toml:"token_budget"`
	WALMaxEntries    int    `toml:"wal_max_entries"`
	ArchiveAfterDays int    `toml:"archive_after_days"`
}

// CompactionConfig holds compaction strategy and weights.
type CompactionConfig struct {
	Strategy      string                `toml:"strategy"`
	PreserveTypes []string              `toml:"preserve_types"`
	DecayTypes    []string              `toml:"decay_types"`
	TypeWeights   CompactionTypeWeights `toml:"type_weights"`
	Decay         DecayConfig           `toml:"decay"`
}

// CompactionTypeWeights maps memory types to their weight multipliers.
type CompactionTypeWeights struct {
	Preference float64 `toml:"preference"`
	Semantic   float64 `toml:"semantic"`
	Procedural float64 `toml:"procedural"`
	Episodic   float64 `toml:"episodic"`
	Code       float64 `toml:"code"`
}

// DecayConfig controls time-based decay for episodic entries.
type DecayConfig struct {
	Rate     float64 `toml:"rate"`
	MinScore float64 `toml:"min_score"`
}

// KnowledgeConfig controls knowledge indexing.
type KnowledgeConfig struct {
	Enabled       bool     `toml:"enabled"`
	ScanPaths     []string `toml:"scan_paths"`
	ExtensionDirs bool     `toml:"extension_dirs"`
	BudgetShare   float64  `toml:"budget_share"`
}

// HooksConfig controls git hooks.
type HooksConfig struct {
	PreCommit bool `toml:"pre_commit"`
}

// Default returns a Config with sane defaults matching the design doc.
func Default() Config {
	return Config{
		Memory: MemoryConfig{
			SchemaVersion:    "1.0",
			TokenBudget:      constants.DefaultTokenBudget,
			WALMaxEntries:    constants.DefaultWALMaxEntries,
			ArchiveAfterDays: constants.DefaultArchiveAfterDays,
		},
		Compaction: CompactionConfig{
			Strategy:      "relevance_scored",
			PreserveTypes: []string{"semantic", "procedural", "preference"},
			DecayTypes:    []string{"episodic"},
			TypeWeights: CompactionTypeWeights{
				Preference: constants.WeightPreference,
				Semantic:   constants.WeightSemantic,
				Procedural: constants.WeightProcedural,
				Code:       constants.WeightCode,
				Episodic:   constants.WeightEpisodic,
			},
			Decay: DecayConfig{
				Rate:     constants.DefaultDecayRate,
				MinScore: constants.DefaultDecayMinScore,
			},
		},
		Knowledge: KnowledgeConfig{
			Enabled: true,
			ScanPaths: []string{
				".github/**/*.md",
				"CLAUDE.md",
				".cursorrules",
				".windsurfrules",
				"**/SKILL.md",
				"**/*.instructions.md",
				"**/*.rules.md",
				"CONTRIBUTING.md",
			},
			ExtensionDirs: true,
			BudgetShare:   0.4,
		},
		Hooks: HooksConfig{
			PreCommit: true,
		},
	}
}

// Load reads config from a TOML file, falling back to defaults for missing fields.
func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Default(), err
	}

	return cfg, nil
}

// Save writes the config to a TOML file.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

// TypeWeight returns the compaction weight for a given memory type string.
func (c *Config) TypeWeight(t string) float64 {
	switch t {
	case "s", "semantic":
		return c.Compaction.TypeWeights.Semantic
	case "e", "episodic":
		return c.Compaction.TypeWeights.Episodic
	case "p", "procedural":
		return c.Compaction.TypeWeights.Procedural
	case "f", "preference":
		return c.Compaction.TypeWeights.Preference
	case "c", "code":
		return c.Compaction.TypeWeights.Code
	default:
		return 0.5
	}
}
