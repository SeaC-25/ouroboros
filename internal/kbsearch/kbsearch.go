// Package kbsearch provides read-only retrieval over the KaliMCP knowledge
// base (data/kb_vectors.db, 265 chunks: attack chains / writeups / recipes).
//
// The vector tables need sqlite-vec which is NOT available in pure Go, so
// this reader uses the plain `chunks` table with keyword matching — no
// embeddings required. Chunks carry (id, source, section, text, category);
// we rank by term-hit density, which is enough for the Progress gate's
// "failure-as-a-query" lookups (WAF type → historical bypass recipes).
package kbsearch

import (
	"database/sql"
	"fmt"
	"strings"
)

// Hit is one ranked chunk.
type Hit struct {
	ChunkID  int64
	Source   string
	Section  string
	Category string
	Text     string
	Score    float64
}

// Reader wraps an opened kb_vectors.db.
type Reader struct {
	db *sql.DB
}

// Open attaches to the knowledge base file. Missing file is an explicit
// error (callers decide whether KB is optional for them).
func Open(path string) (*Reader, error) {
	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("kb open: %w", err)
	}
	// Probe the expected table.
	var one int
	if err := db.QueryRow(`SELECT 1 FROM chunks LIMIT 1`).Scan(&one); err != nil {
		db.Close()
		return nil, fmt.Errorf("kb probe (chunks table missing?): %w", err)
	}
	return &Reader{db: db}, nil
}

// Close releases the handle.
func (r *Reader) Close() error { return r.db.Close() }

// Search ranks chunks containing ALL terms (case-insensitive substring),
// by hit density (hits / text length — short dense chunks win).
// Returns up to limit hits, best first.
func (r *Reader) Search(terms []string, limit int) ([]Hit, error) {
	clean := make([]string, 0, len(terms))
	for _, t := range terms {
		if t = strings.TrimSpace(strings.ToLower(t)); t != "" {
			clean = append(clean, t)
		}
	}
	if len(clean) == 0 {
		return nil, fmt.Errorf("kb search: no terms")
	}
	if limit <= 0 {
		limit = 3
	}

	rows, err := r.db.Query(`SELECT id, source, section, text, category FROM chunks`)
	if err != nil {
		return nil, fmt.Errorf("kb query: %w", err)
	}
	defer rows.Close()

	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ChunkID, &h.Source, &h.Section, &h.Text, &h.Category); err != nil {
			return nil, fmt.Errorf("kb scan: %w", err)
		}
		lower := strings.ToLower(h.Text)
		total := 0
		ok := true
		for _, t := range clean {
			n := strings.Count(lower, t)
			if n == 0 {
				ok = false
				break
			}
			total += n
		}
		if !ok {
			continue
		}
		denom := float64(len(h.Text))
		if denom < 1 {
			denom = 1
		}
		h.Score = float64(total) / denom
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kb iterate: %w", err)
	}

	// Top-limit by score (insertion sort, N is small).
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Score > hits[j-1].Score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}
