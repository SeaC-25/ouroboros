package toolchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusSearchByKeywordReturnsSnippetAndArtifactRef(t *testing.T) {
	corpus, err := OpenCorpus(filepath.Join("..", "..", "test", "fixtures", "corpus"))
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}

	results, err := corpus.Search(CorpusSearchRequest{
		CorpusName: "nuclei",
		Query:      "exposure",
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("search corpus: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	result := results[0]
	if result.ArtifactRef.CorpusName != "nuclei" {
		t.Fatalf("unexpected corpus name: %s", result.ArtifactRef.CorpusName)
	}
	if result.ArtifactRef.Path != "http/test-template.yaml" {
		t.Fatalf("unexpected artifact path: %s", result.ArtifactRef.Path)
	}
	if result.ArtifactRef.Digest == "" {
		t.Fatalf("expected artifact digest")
	}
	if result.ArtifactRef.Size == 0 {
		t.Fatalf("expected artifact size")
	}
	if !strings.Contains(strings.ToLower(result.Snippet), "exposure") {
		t.Fatalf("expected snippet to include query, got %q", result.Snippet)
	}
	if result.Confidence <= 0 {
		t.Fatalf("expected positive confidence, got %f", result.Confidence)
	}
}

func TestCorpusReadRejectsPathTraversal(t *testing.T) {
	corpus, err := OpenCorpus(filepath.Join("..", "..", "test", "fixtures", "corpus"))
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}

	for _, corpusPath := range []string{"../secret.yaml", "http/../secret.yaml"} {
		_, err = corpus.Read("nuclei", corpusPath)
		if err == nil {
			t.Fatalf("expected path traversal %q to fail", corpusPath)
		}
		if !strings.Contains(err.Error(), "invalid corpus path") {
			t.Fatalf("expected invalid path error, got %v", err)
		}
	}
}

func TestCorpusSearchBoundsResults(t *testing.T) {
	root := t.TempDir()
	corpusDir := filepath.Join(root, "payloads")
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		t.Fatalf("create corpus fixture: %v", err)
	}
	for _, name := range []string{"one.txt", "two.txt", "three.txt"} {
		if err := os.WriteFile(filepath.Join(corpusDir, name), []byte("shared keyword template\n"), 0o644); err != nil {
			t.Fatalf("write corpus fixture: %v", err)
		}
	}

	corpus, err := OpenCorpus(root)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}

	results, err := corpus.Search(CorpusSearchRequest{
		CorpusName: "payloads",
		Query:      "template",
		MaxResults: 1,
	})
	if err != nil {
		t.Fatalf("search corpus: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected bounded result count of one, got %d", len(results))
	}
}
