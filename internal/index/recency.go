package index

import (
	"encoding/json"
	"os"
)

const recencyRingSize = 256

// RecencyRing is a fixed-size LRU ring buffer of recently accessed entry IDs.
type RecencyRing struct {
	IDs []string `json:"ids"`
}

// NewRecencyRing creates an empty recency ring.
func NewRecencyRing() *RecencyRing {
	return &RecencyRing{IDs: make([]string, 0, recencyRingSize)}
}

// Touch moves an ID to the front of the ring (most recent).
func (r *RecencyRing) Touch(id string) {
	// Remove existing occurrence
	for i, existing := range r.IDs {
		if existing == id {
			r.IDs = append(r.IDs[:i], r.IDs[i+1:]...)
			break
		}
	}

	// Prepend
	r.IDs = append([]string{id}, r.IDs...)

	// Trim to max size
	if len(r.IDs) > recencyRingSize {
		r.IDs = r.IDs[:recencyRingSize]
	}
}

// Position returns the position of an ID in the ring (0 = most recent).
// Returns -1 if not found.
func (r *RecencyRing) Position(id string) int {
	for i, existing := range r.IDs {
		if existing == id {
			return i
		}
	}
	return -1
}

// RecencyBoost returns a boost factor based on position in the ring.
// Most recent entries get the highest boost (1.0), decaying linearly.
func (r *RecencyRing) RecencyBoost(id string) float64 {
	pos := r.Position(id)
	if pos < 0 {
		return 0
	}
	return 1.0 - float64(pos)/float64(recencyRingSize)
}

// Load reads the recency ring from a JSON file.
func (r *RecencyRing) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, r)
}

// Save writes the recency ring to a JSON file.
func (r *RecencyRing) Save(path string) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// TagMap maps tags to sets of entry IDs for O(1) tag filtering.
type TagMap struct {
	Tags map[string][]string `json:"tags"`
}

// NewTagMap creates an empty tag map.
func NewTagMap() *TagMap {
	return &TagMap{Tags: make(map[string][]string)}
}

// Add associates an entry ID with the given tags.
func (tm *TagMap) Add(id string, tags []string) {
	for _, tag := range tags {
		tm.Tags[tag] = append(tm.Tags[tag], id)
	}
}

// Lookup returns all entry IDs associated with a tag.
func (tm *TagMap) Lookup(tag string) []string {
	return tm.Tags[tag]
}

// Load reads the tag map from a JSON file.
func (tm *TagMap) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, tm)
}

// Save writes the tag map to a JSON file.
func (tm *TagMap) Save(path string) error {
	data, err := json.Marshal(tm)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
