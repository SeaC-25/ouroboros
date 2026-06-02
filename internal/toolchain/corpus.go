package toolchain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultCorpusMaxResults = 10
	hardCorpusMaxResults    = 50
	corpusSnippetRadius     = 120
)

type Corpus struct {
	root string
}

type CorpusSearchRequest struct {
	CorpusName string
	Query      string
	PathFilter string
	MaxResults int
}

type CorpusArtifactRef struct {
	CorpusName string
	Path       string
	Digest     string
	Size       int64
	MediaType  string
}

type CorpusSearchResult struct {
	ArtifactRef CorpusArtifactRef
	Snippet     string
	Confidence  float64
}

type CorpusDocument struct {
	ArtifactRef CorpusArtifactRef
	Content     []byte
}

func OpenCorpus(root string) (*Corpus, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("corpus root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve corpus root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat corpus root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("corpus root is not a directory: %s", root)
	}
	return &Corpus{root: absRoot}, nil
}

func (c *Corpus) Search(request CorpusSearchRequest) ([]CorpusSearchResult, error) {
	corpusName, err := cleanCorpusName(request.CorpusName)
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, errors.New("corpus search query is required")
	}
	filter, err := cleanOptionalCorpusPath(request.PathFilter)
	if err != nil {
		return nil, err
	}
	limit := corpusResultLimit(request.MaxResults)
	corpusRoot := filepath.Join(c.root, filepath.FromSlash(corpusName))
	if err := c.ensureInsideRoot(corpusRoot); err != nil {
		return nil, err
	}

	var results []CorpusSearchResult
	lowerQuery := strings.ToLower(query)
	err = filepath.WalkDir(corpusRoot, func(filePath string, dirEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if dirEntry.IsDir() {
			return nil
		}
		if dirEntry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := dirEntry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relPath, err := filepath.Rel(corpusRoot, filePath)
		if err != nil {
			return err
		}
		corpusPath := filepath.ToSlash(relPath)
		if filter != "" && !strings.Contains(corpusPath, filter) {
			return nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		contentMatch := strings.Contains(strings.ToLower(string(data)), lowerQuery)
		pathMatch := strings.Contains(strings.ToLower(corpusPath), lowerQuery)
		if !contentMatch && !pathMatch {
			return nil
		}
		confidence := 0.70
		if contentMatch {
			confidence = 1.0
		}
		results = append(results, CorpusSearchResult{
			ArtifactRef: corpusRef(corpusName, corpusPath, data),
			Snippet:     corpusSnippet(data, query),
			Confidence:  confidence,
		})
		if len(results) >= limit {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("search corpus %s: %w", corpusName, err)
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].ArtifactRef.Path < results[j].ArtifactRef.Path
	})
	return results, nil
}

func (c *Corpus) Read(corpusName string, corpusPath string) (CorpusDocument, error) {
	cleanName, err := cleanCorpusName(corpusName)
	if err != nil {
		return CorpusDocument{}, err
	}
	cleanPath, err := cleanCorpusPath(corpusPath)
	if err != nil {
		return CorpusDocument{}, err
	}
	corpusRoot := filepath.Join(c.root, filepath.FromSlash(cleanName))
	if err := c.ensureInsideRoot(corpusRoot); err != nil {
		return CorpusDocument{}, err
	}
	filePath := filepath.Join(corpusRoot, filepath.FromSlash(cleanPath))
	if err := ensurePathInside(corpusRoot, filePath); err != nil {
		return CorpusDocument{}, err
	}
	info, err := os.Lstat(filePath)
	if err != nil {
		return CorpusDocument{}, fmt.Errorf("stat corpus artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return CorpusDocument{}, fmt.Errorf("invalid corpus path: %s", corpusPath)
	}
	if !info.Mode().IsRegular() {
		return CorpusDocument{}, fmt.Errorf("corpus artifact is not a regular file: %s", corpusPath)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return CorpusDocument{}, fmt.Errorf("read corpus artifact: %w", err)
	}
	return CorpusDocument{
		ArtifactRef: corpusRef(cleanName, cleanPath, data),
		Content:     data,
	}, nil
}

func cleanCorpusName(value string) (string, error) {
	cleaned, err := cleanCorpusPath(value)
	if err != nil {
		return "", err
	}
	if strings.Contains(cleaned, "/") {
		return "", fmt.Errorf("invalid corpus name: %s", value)
	}
	return cleaned, nil
}

func cleanCorpusPath(value string) (string, error) {
	cleaned, err := cleanOptionalCorpusPath(value)
	if err != nil {
		return "", err
	}
	if cleaned == "" {
		return "", errors.New("invalid corpus path: path is required")
	}
	return cleaned, nil
}

func cleanOptionalCorpusPath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("invalid corpus path: %s", value)
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", fmt.Errorf("invalid corpus path: %s", value)
		}
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid corpus path: %s", value)
	}
	return cleaned, nil
}

func corpusResultLimit(maxResults int) int {
	if maxResults <= 0 {
		return defaultCorpusMaxResults
	}
	if maxResults > hardCorpusMaxResults {
		return hardCorpusMaxResults
	}
	return maxResults
}

func (c *Corpus) ensureInsideRoot(target string) error {
	return ensurePathInside(c.root, target)
}

func ensurePathInside(root string, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("resolve corpus path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid corpus path: %s", target)
	}
	return nil
}

func corpusRef(corpusName string, corpusPath string, data []byte) CorpusArtifactRef {
	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	mediaType := mime.TypeByExtension(filepath.Ext(corpusPath))
	if mediaType == "" && (strings.HasSuffix(corpusPath, ".yaml") || strings.HasSuffix(corpusPath, ".yml")) {
		mediaType = "application/x-yaml"
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return CorpusArtifactRef{
		CorpusName: corpusName,
		Path:       corpusPath,
		Digest:     digest,
		Size:       int64(len(data)),
		MediaType:  mediaType,
	}
}

func corpusSnippet(data []byte, query string) string {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return ""
	}
	lowerText := strings.ToLower(text)
	index := strings.Index(lowerText, strings.ToLower(query))
	if index < 0 {
		lineEnd := strings.IndexByte(text, '\n')
		if lineEnd < 0 {
			return compactWhitespace(text)
		}
		return compactWhitespace(text[:lineEnd])
	}
	start := index - corpusSnippetRadius
	if start < 0 {
		start = 0
	}
	end := index + len(query) + corpusSnippetRadius
	if end > len(text) {
		end = len(text)
	}
	return compactWhitespace(text[start:end])
}

func compactWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
