package ai

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
)

// HashEmbedder is a dependency-free, deterministic embedder for local
// development when no real embedding provider is configured. It hashes words
// into a bag-of-words vector: it captures word overlap, not true semantics, so
// it is NOT for production — use a real provider (e.g. Gemini) there.
type HashEmbedder struct {
	Dim int // vector dimension; defaults to 768 to match the faqs column
}

func (h HashEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	dim := h.Dim
	if dim == 0 {
		dim = 768
	}

	vec := make([]float32, dim)
	for _, tok := range strings.Fields(strings.ToLower(text)) {
		sum := fnv.New32a()
		_, _ = sum.Write([]byte(tok))
		vec[sum.Sum32()%uint32(dim)]++
	}

	// L2-normalize so cosine similarity is well-defined.
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		inv := float32(1 / math.Sqrt(norm))
		for i := range vec {
			vec[i] *= inv
		}
	}
	return vec, nil
}
