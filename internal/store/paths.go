package store

import (
	"os"
	"path/filepath"
)

const (
	DirName         = ".memor"
	MemoryDBFile    = "memory.db"
	MemoryWALFile   = "memory.wal"
	ArchiveFile     = "memory.archive"
	KnowledgeDBFile = "knowledge.db"
	ConfigFile      = "config.toml"
	IndexDir        = "index"
	TrigramsFile    = "trigrams.bin"
	TagsFile        = "tags.json"
	BloomFile       = "bloom.bin"
	RecencyFile     = "recency.json"
)

// Paths holds resolved paths to all memor files for a project.
type Paths struct {
	Root      string // .memor/ directory
	MemoryDB  string
	MemoryWAL string
	Archive   string
	Knowledge string
	Config    string
	Index     string
	Trigrams  string
	Tags      string
	Bloom     string
	Recency   string
}

// ResolvePaths computes all paths relative to a project root.
func ResolvePaths(projectRoot string) Paths {
	root := filepath.Join(projectRoot, DirName)
	idx := filepath.Join(root, IndexDir)
	return Paths{
		Root:      root,
		MemoryDB:  filepath.Join(root, MemoryDBFile),
		MemoryWAL: filepath.Join(root, MemoryWALFile),
		Archive:   filepath.Join(root, ArchiveFile),
		Knowledge: filepath.Join(root, KnowledgeDBFile),
		Config:    filepath.Join(root, ConfigFile),
		Index:     idx,
		Trigrams:  filepath.Join(idx, TrigramsFile),
		Tags:      filepath.Join(idx, TagsFile),
		Bloom:     filepath.Join(idx, BloomFile),
		Recency:   filepath.Join(idx, RecencyFile),
	}
}

// ResolveUserPaths computes paths for the user-global memory in ~/.memor/.
func ResolveUserPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	return ResolvePaths(home), nil
}

// EnsureDirs creates the .memor/ and .memor/index/ directories if they don't exist.
func (p *Paths) EnsureDirs() error {
	if err := os.MkdirAll(p.Root, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(p.Index, 0o755)
}

// Exists returns true if the .memor/ directory exists.
func (p *Paths) Exists() bool {
	info, err := os.Stat(p.Root)
	return err == nil && info.IsDir()
}
