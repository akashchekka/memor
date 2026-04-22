package index

import (
	"testing"
)

func TestTrigrams(t *testing.T) {
	tris := Trigrams("deploy")
	if len(tris) == 0 {
		t.Fatal("expected trigrams for 'deploy'")
	}

	expected := map[string]bool{"dep": true, "epl": true, "plo": true, "loy": true}
	for _, tri := range tris {
		if !expected[tri] {
			t.Errorf("unexpected trigram: %s", tri)
		}
	}
}

func TestTrigrams_Short(t *testing.T) {
	tris := Trigrams("ab")
	if len(tris) != 0 {
		t.Error("strings shorter than 3 chars should produce no trigrams")
	}
}

func TestTrigrams_Dedup(t *testing.T) {
	tris := Trigrams("aaaa")
	if len(tris) != 1 {
		t.Errorf("expected 1 unique trigram for 'aaaa', got %d", len(tris))
	}
}

func TestTrigramIndex_SearchFindsMatch(t *testing.T) {
	idx := NewTrigramIndex()
	idx.Add(0, "PostgreSQL database with Drizzle ORM")
	idx.Add(1, "Deploy API to production with Railway")
	idx.Add(2, "Redis cache for session storage")

	results := idx.Search("database")
	if len(results) == 0 {
		t.Fatal("expected to find 'database' match")
	}
	if results[0] != 0 {
		t.Errorf("expected doc 0, got %d", results[0])
	}
}

func TestTrigramIndex_SearchNoMatch(t *testing.T) {
	idx := NewTrigramIndex()
	idx.Add(0, "PostgreSQL database")

	results := idx.Search("kubernetes")
	if len(results) != 0 {
		t.Errorf("expected no matches, got %d", len(results))
	}
}

func TestBM25_RanksRelevantHigher(t *testing.T) {
	docs := []string{
		"postgresql database drizzle orm migrations",
		"deploy api production railway hosting",
		"redis cache session ttl storage",
	}

	scorer := NewBM25Scorer(docs, DefaultBM25Params())

	dbScore := scorer.Score(0, "database migration")
	deployScore := scorer.Score(1, "database migration")

	if dbScore <= deployScore {
		t.Errorf("doc about database should score higher for 'database migration': db=%.3f deploy=%.3f",
			dbScore, deployScore)
	}
}

func TestRecencyRing_Touch(t *testing.T) {
	r := NewRecencyRing()
	r.Touch("aaa")
	r.Touch("bbb")
	r.Touch("ccc")

	if r.IDs[0] != "ccc" {
		t.Errorf("most recent should be 'ccc', got %s", r.IDs[0])
	}

	// Touch "aaa" again — should move to front
	r.Touch("aaa")
	if r.IDs[0] != "aaa" {
		t.Errorf("after re-touch, 'aaa' should be first, got %s", r.IDs[0])
	}
	if len(r.IDs) != 3 {
		t.Errorf("expected 3 entries, got %d", len(r.IDs))
	}
}

func TestRecencyRing_Boost(t *testing.T) {
	r := NewRecencyRing()
	r.Touch("old")
	r.Touch("new")

	newBoost := r.RecencyBoost("new")
	oldBoost := r.RecencyBoost("old")
	missingBoost := r.RecencyBoost("missing")

	if newBoost <= oldBoost {
		t.Error("newer entry should have higher boost")
	}
	if missingBoost != 0 {
		t.Error("missing entry should have zero boost")
	}
}
