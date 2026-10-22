package kbsearch

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// buildTestDB creates a minimal kb_vectors.db with the chunks table.
func buildTestDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kb_vectors.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	schema := `CREATE TABLE chunks (
		id INTEGER PRIMARY KEY,
		source TEXT, section TEXT, text TEXT, category TEXT
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	seed := [][]string{
		{"writeup", "waf-bypass", "CloudFront 403 on quote — bypass via JSON boolean type confusion returned 500", "writeup"},
		{"writeup", "auth-chain", "leaked APK signing key plus archive route reused admin session", "writeup"},
		{"chain", "sqli-to-rce", "sql injection union select into outfile then webshell", "chain"},
		{"recipe", "nmap", "nmap -sV -p 80,443 target", "recipe"},
	}
	for i, s := range seed {
		if _, err := db.Exec(`INSERT INTO chunks (id, source, section, text, category) VALUES (?,?,?,?,?)`,
			i+1, s[0], s[1], s[2], s[3]); err != nil {
			t.Fatalf("seed %d: %v", i+1, err)
		}
	}
	_ = db.Close()
	return path
}

func TestSearchRanksAndFilters(t *testing.T) {
	path := buildTestDB(t)
	r, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()

	hits, err := r.Search([]string{"cloudfront", "bypass"}, 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected the waf-bypass writeup to match")
	}
	if hits[0].Section != "waf-bypass" {
		t.Fatalf("best hit should be waf-bypass, got %s", hits[0].Section)
	}

	hits, err = r.Search([]string{"nonexistent-term-xyz"}, 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("no-match search must return empty, got %d", len(hits))
	}
}

func TestSearchAllTermsRequired(t *testing.T) {
	r, err := Open(buildTestDB(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()

	// "cloudfront" only exists with "bypass"; searching cloudfront + nmap
	// (no chunk has both) must return zero hits.
	hits, err := r.Search([]string{"cloudfront", "nmap"}, 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("AND semantics violated: %d hits for disjoint terms", len(hits))
	}
}

func TestOpenMissingFileIsExplicitError(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "absent.db")); err == nil {
		t.Fatal("missing file must be an explicit error")
	}
}
