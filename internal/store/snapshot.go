package store

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/memor-dev/memor/internal/memory"
	"github.com/memor-dev/memor/internal/token"
)

// Snapshot represents the parsed content of memory.db.
type Snapshot struct {
	Version     string
	EntryCount  int
	TokenBudget int
	CompactedAt time.Time
	Entries     []memory.Entry
}

var headerRegex = regexp.MustCompile(
	`^@mem v(\S+) \| (\d+) entries \| budget:(\d+) \| compacted:(.+)$`,
)

var entryRegex = regexp.MustCompile(
	`^(@[sepf])\s+((?:#\S+\s*)+):\s+(.+?)\s+\[([^\]]+)\]$`,
)

// ReadSnapshot parses memory.db from the compact DSL format.
func ReadSnapshot(path string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Snapshot{Version: "1"}, nil
		}
		return nil, fmt.Errorf("open snapshot: %w", err)
	}
	defer f.Close()

	snap := &Snapshot{Version: "1"}
	scanner := bufio.NewScanner(f)

	// Parse header
	if scanner.Scan() {
		line := scanner.Text()
		if m := headerRegex.FindStringSubmatch(line); m != nil {
			snap.Version = m[1]
			fmt.Sscanf(m[2], "%d", &snap.EntryCount)
			fmt.Sscanf(m[3], "%d", &snap.TokenBudget)
			snap.CompactedAt, _ = time.Parse(time.RFC3339, m[4])
		}
	}

	// Parse entries
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		m := entryRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		prefix := m[1]
		tagStr := m[2]
		content := m[3]
		datestamp := m[4]

		entry := memory.Entry{
			Type:    prefixToType(prefix),
			Content: content,
			ID:      memory.ContentID(content),
			Tags:    parseTags(tagStr),
		}

		if datestamp == "perm" {
			entry.Expires = -1 // sentinel for permanent
		} else {
			if t, err := time.Parse("2006-01-02", datestamp); err == nil {
				entry.Timestamp = t.Unix()
			}
		}

		snap.Entries = append(snap.Entries, entry)
	}

	return snap, scanner.Err()
}

// WriteSnapshot renders entries to memory.db in compact DSL format within the token budget.
func WriteSnapshot(path string, entries []memory.Entry, budget int) error {
	// Sort: by type order, then within type by timestamp descending
	sort.SliceStable(entries, func(i, j int) bool {
		ti, tj := entries[i].Type.SortOrder(), entries[j].Type.SortOrder()
		if ti != tj {
			return ti < tj
		}
		return entries[i].Timestamp > entries[j].Timestamp
	})

	now := time.Now().UTC().Format(time.RFC3339)

	var lines []string
	tokenCount := 0

	for _, e := range entries {
		line := renderEntry(e)
		lineTokens := token.Count(line)
		if tokenCount+lineTokens > budget {
			break
		}
		lines = append(lines, line)
		tokenCount += lineTokens
	}

	header := fmt.Sprintf("@mem v1 | %d entries | budget:%d | compacted:%s",
		len(lines), budget, now)
	headerTokens := token.Count(header)

	// Trim entries if header pushes over budget
	for tokenCount+headerTokens > budget && len(lines) > 0 {
		last := lines[len(lines)-1]
		tokenCount -= token.Count(last)
		lines = lines[:len(lines)-1]
	}

	// Re-render header with final count
	header = fmt.Sprintf("@mem v1 | %d entries | budget:%d | compacted:%s",
		len(lines), budget, now)

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n\n")
	for _, line := range lines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// renderEntry formats a single entry as a compact DSL line.
func renderEntry(e memory.Entry) string {
	prefix := e.Type.Prefix()
	tags := renderTags(e.Tags)
	datestamp := formatDatestamp(e)
	return fmt.Sprintf("%s %s: %s [%s]", prefix, tags, e.Content, datestamp)
}

func formatDatestamp(e memory.Entry) string {
	if e.Type == memory.TypePreference || e.Expires == -1 {
		return "perm"
	}
	if e.Timestamp == 0 {
		return time.Now().Format("2006-01-02")
	}
	return time.Unix(e.Timestamp, 0).Format("2006-01-02")
}

func renderTags(tags []string) string {
	var parts []string
	for _, t := range tags {
		parts = append(parts, "#"+t)
	}
	return strings.Join(parts, " ")
}

func parseTags(s string) []string {
	var tags []string
	for _, part := range strings.Fields(s) {
		tag := strings.TrimPrefix(part, "#")
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func prefixToType(prefix string) memory.Type {
	switch prefix {
	case "@s":
		return memory.TypeSemantic
	case "@e":
		return memory.TypeEpisodic
	case "@p":
		return memory.TypeProcedural
	case "@f":
		return memory.TypePreference
	default:
		return memory.TypeSemantic
	}
}
