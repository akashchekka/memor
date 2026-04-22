package index

import (
	"os"

	"github.com/bits-and-blooms/bloom/v3"
)

const (
	bloomExpectedItems = 10000
	bloomFPRate        = 0.01
)

// BloomIndex wraps a Bloom filter for fast negative lookups.
type BloomIndex struct {
	filter *bloom.BloomFilter
}

// NewBloomIndex creates a fresh Bloom filter sized for the expected item count.
func NewBloomIndex() *BloomIndex {
	return &BloomIndex{
		filter: bloom.NewWithEstimates(bloomExpectedItems, bloomFPRate),
	}
}

// Add inserts all trigrams of the text into the Bloom filter.
func (b *BloomIndex) Add(text string) {
	for _, tri := range Trigrams(text) {
		b.filter.AddString(tri)
	}
}

// MayContain returns true if the query trigrams might exist in the index.
// A false return means "definitely not present."
func (b *BloomIndex) MayContain(query string) bool {
	for _, tri := range Trigrams(query) {
		if !b.filter.TestString(tri) {
			return false
		}
	}
	return true
}

// Save writes the Bloom filter to a binary file.
func (b *BloomIndex) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = b.filter.WriteTo(f)
	return err
}

// Load reads the Bloom filter from a binary file.
func (b *BloomIndex) Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			b.filter = bloom.NewWithEstimates(bloomExpectedItems, bloomFPRate)
			return nil
		}
		return err
	}
	defer f.Close()
	b.filter = bloom.NewWithEstimates(bloomExpectedItems, bloomFPRate)
	_, err = b.filter.ReadFrom(f)
	return err
}
