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
	`^(@[sepfc])\s+((?:#\S+\s*)+):\s+(.+?)\s+\[([^\]]+)\]$`,
)

var codeEntryRegex = regexp.MustCompile(
	`^@c\s+(\S+)\s+\[(\d+)\s+LOC\s+\|\s+([a-f0-9]+)\]$`,
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

	// Collect all remaining lines
	var allLines []string
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	// Parse entries from lines
	for i := 0; i < len(allLines); i++ {
		line := strings.TrimSpace(allLines[i])
		if line == "" {
			continue
		}

		// Check for @c (code) multi-line entries
		if cm := codeEntryRegex.FindStringSubmatch(line); cm != nil {
			filePath := cm[1]
			loc := 0
			fmt.Sscanf(cm[2], "%d", &loc)
			hash := cm[3]

			meta := &memory.CodeMeta{
				FilePath: filePath,
				LOC:      loc,
				Hash:     hash,
			}

			// Read continuation lines (indented with "  key: value")
			for i+1 < len(allLines) && strings.HasPrefix(allLines[i+1], "  ") {
				i++
				sub := strings.TrimSpace(allLines[i])
				if kv := strings.SplitN(sub, ": ", 2); len(kv) == 2 {
					switch kv[0] {
					case "exports":
						for _, e := range strings.Split(kv[1], ", ") {
							e = strings.TrimSpace(e)
							if e != "" {
								meta.Exports = append(meta.Exports, e)
							}
						}
					case "deps":
						for _, d := range strings.Split(kv[1], ", ") {
							d = strings.TrimSpace(d)
							if d != "" {
								meta.Deps = append(meta.Deps, d)
							}
						}
					case "summary":
						meta.Summary = kv[1]
					case "patterns":
						meta.Patterns = kv[1]
					case "logic":
						meta.Logic = kv[1]
					}
				}
			}

			entry := memory.Entry{
				Type:      memory.TypeCode,
				Content:   filePath,
				ID:        memory.ContentID(filePath),
				Timestamp: time.Now().Unix(),
				Meta:      meta,
			}
			snap.Entries = append(snap.Entries, entry)
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
	// Sort: by timestamp descending (newest first), then by type order as tiebreaker
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Timestamp != entries[j].Timestamp {
			return entries[i].Timestamp > entries[j].Timestamp
		}
		return entries[i].Type.SortOrder() < entries[j].Type.SortOrder()
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

// renderEntry formats a single entry as a compact DSL line (or multi-line block for @c).
func renderEntry(e memory.Entry) string {
	if e.Type == memory.TypeCode && e.Meta != nil {
		return renderCodeSnapshotEntry(e)
	}
	prefix := e.Type.Prefix()
	tags := renderTags(e.Tags)
	datestamp := formatDatestamp(e)
	return fmt.Sprintf("%s %s: %s [%s]", prefix, tags, e.Content, datestamp)
}

// renderCodeSnapshotEntry formats a @c entry as a multi-line block for the snapshot.
func renderCodeSnapshotEntry(e memory.Entry) string {
	m := e.Meta
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("@c %s [%d LOC | %s]", m.FilePath, m.LOC, m.Hash))
	if len(m.Exports) > 0 {
		sb.WriteString(fmt.Sprintf("\n  exports: %s", strings.Join(m.Exports, ", ")))
	}
	if len(m.Deps) > 0 {
		sb.WriteString(fmt.Sprintf("\n  deps: %s", strings.Join(m.Deps, ", ")))
	}
	if m.Summary != "" {
		sb.WriteString(fmt.Sprintf("\n  summary: %s", m.Summary))
	}
	if m.Patterns != "" {
		sb.WriteString(fmt.Sprintf("\n  patterns: %s", m.Patterns))
	}
	if m.Logic != "" {
		sb.WriteString(fmt.Sprintf("\n  logic: %s", m.Logic))
	}
	return sb.String()
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
	case "@c":
		return memory.TypeCode
	default:
		return memory.TypeSemantic
	}
}
