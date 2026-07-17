package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// Embedder turns text into a fixed-dimension vector. One embedding provider is
// used for the whole index: vectors from different models are not comparable.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// FAQs is the FAQ knowledge base: creation (with embeddings) and RAG search.
// Its Search method implements core.Retriever.
type FAQs struct {
	pool     *pgxpool.Pool
	embedder Embedder
}

// NewFAQs constructs a FAQs store.
func NewFAQs(pool *pgxpool.Pool, embedder Embedder) *FAQs {
	return &FAQs{pool: pool, embedder: embedder}
}

// Create embeds the question and inserts a new FAQ, returning its id.
func (f *FAQs) Create(ctx context.Context, businessID int64, question, answer, category string) (int64, error) {
	emb, err := f.embedder.Embed(ctx, question)
	if err != nil {
		return 0, fmt.Errorf("embed question: %w", err)
	}

	const q = `
		INSERT INTO faqs (business_id, question, answer, embedding, category)
		VALUES ($1, $2, $3, $4::vector, $5)
		RETURNING id`

	var id int64
	if err := f.pool.QueryRow(ctx, q,
		businessID, question, answer, encodeVector(emb), nullIfEmpty(category),
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("insert faq: %w", err)
	}
	return id, nil
}

// Search embeds the query and returns the most similar FAQs for the business,
// ordered by descending cosine similarity. The business_id filter enforces
// tenant isolation: a business can never match another's FAQs.
func (f *FAQs) Search(ctx context.Context, businessID int64, query string, limit int) ([]core.Match, error) {
	emb, err := f.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	const q = `
		SELECT id, question, answer, 1 - (embedding <=> $2::vector) AS score
		FROM faqs
		WHERE business_id = $1 AND embedding IS NOT NULL
		ORDER BY embedding <=> $2::vector
		LIMIT $3`

	rows, err := f.pool.Query(ctx, q, businessID, encodeVector(emb), limit)
	if err != nil {
		return nil, fmt.Errorf("faq search: %w", err)
	}
	defer rows.Close()

	var matches []core.Match
	for rows.Next() {
		var m core.Match
		if err := rows.Scan(&m.FAQID, &m.Question, &m.Answer, &m.Score); err != nil {
			return nil, fmt.Errorf("scan match: %w", err)
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate matches: %w", err)
	}
	return matches, nil
}

// FAQ is a stored FAQ without its embedding (for admin listing).
type FAQ struct {
	ID       int64
	Question string
	Answer   string
	Category string
}

// List returns a business's FAQs, most recently updated first.
func (f *FAQs) List(ctx context.Context, businessID int64) ([]FAQ, error) {
	const q = `
		SELECT id, question, answer, coalesce(category, '')
		FROM faqs
		WHERE business_id = $1
		ORDER BY updated_at DESC`

	rows, err := f.pool.Query(ctx, q, businessID)
	if err != nil {
		return nil, fmt.Errorf("list faqs: %w", err)
	}
	defer rows.Close()

	var out []FAQ
	for rows.Next() {
		var it FAQ
		if err := rows.Scan(&it.ID, &it.Question, &it.Answer, &it.Category); err != nil {
			return nil, fmt.Errorf("scan faq: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Categories returns a business's FAQ categories in insertion order (the order
// the knowledge base was authored in), for building menus.
func (f *FAQs) Categories(ctx context.Context, businessID int64) ([]string, error) {
	const q = `
		SELECT category
		FROM faqs
		WHERE business_id = $1 AND category IS NOT NULL AND category <> ''
		GROUP BY category
		ORDER BY min(id)`

	rows, err := f.pool.Query(ctx, q, businessID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListByCategory returns a business's FAQs in one category, oldest first.
func (f *FAQs) ListByCategory(ctx context.Context, businessID int64, category string) ([]FAQ, error) {
	const q = `
		SELECT id, question, answer, coalesce(category, '')
		FROM faqs
		WHERE business_id = $1 AND category = $2
		ORDER BY id`

	rows, err := f.pool.Query(ctx, q, businessID, category)
	if err != nil {
		return nil, fmt.Errorf("list by category: %w", err)
	}
	defer rows.Close()

	var out []FAQ
	for rows.Next() {
		var it FAQ
		if err := rows.Scan(&it.ID, &it.Question, &it.Answer, &it.Category); err != nil {
			return nil, fmt.Errorf("scan faq: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetByID returns one FAQ, scoped to the business (tenant isolation).
func (f *FAQs) GetByID(ctx context.Context, businessID, id int64) (FAQ, error) {
	const q = `
		SELECT id, question, answer, coalesce(category, '')
		FROM faqs
		WHERE business_id = $1 AND id = $2`

	var it FAQ
	if err := f.pool.QueryRow(ctx, q, businessID, id).Scan(&it.ID, &it.Question, &it.Answer, &it.Category); err != nil {
		return FAQ{}, fmt.Errorf("get faq: %w", err)
	}
	return it, nil
}

// Delete removes a FAQ. The business_id filter enforces tenant isolation: a
// business can only delete its own FAQs.
func (f *FAQs) Delete(ctx context.Context, businessID, id int64) error {
	const q = `DELETE FROM faqs WHERE business_id = $1 AND id = $2`
	if _, err := f.pool.Exec(ctx, q, businessID, id); err != nil {
		return fmt.Errorf("delete faq: %w", err)
	}
	return nil
}

// encodeVector formats a vector as pgvector's text form: "[v1,v2,...]".
func encodeVector(v []float32) string {
	b := make([]byte, 0, len(v)*6+2)
	b = append(b, '[')
	for i, f := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendFloat(b, float64(f), 'f', -1, 32)
	}
	b = append(b, ']')
	return string(b)
}

// nullIfEmpty returns nil for an empty string so it is stored as SQL NULL.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
