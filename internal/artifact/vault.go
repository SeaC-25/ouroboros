package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Vault struct {
	root string
}

type Ref struct {
	Digest    string
	Size      int64
	Path      string
	MediaType string
	Name      string
}

func OpenVault(root string) (*Vault, error) {
	if root == "" {
		return nil, errors.New("artifact vault root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact vault: %w", err)
	}
	return &Vault{root: root}, nil
}

func (v *Vault) Write(name string, mediaType string, data []byte) (Ref, error) {
	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	path := v.pathForDigest(digest)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Ref{}, fmt.Errorf("create artifact shard: %w", err)
	}

	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return Ref{}, fmt.Errorf("stat artifact: %w", err)
		}
		tmpPath := path + ".tmp"
		if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
			return Ref{}, fmt.Errorf("write artifact: %w", err)
		}
		if err := os.Rename(tmpPath, path); err != nil {
			_ = os.Remove(tmpPath)
			return Ref{}, fmt.Errorf("commit artifact: %w", err)
		}
	}

	return Ref{
		Digest:    digest,
		Size:      int64(len(data)),
		Path:      path,
		MediaType: mediaType,
		Name:      name,
	}, nil
}

func (v *Vault) Read(digest string) ([]byte, error) {
	if !digestPattern.MatchString(digest) {
		return nil, fmt.Errorf("invalid digest: %s", digest)
	}
	data, err := os.ReadFile(v.pathForDigest(digest))
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	return data, nil
}

func (v *Vault) pathForDigest(digest string) string {
	return filepath.Join(v.root, digest[:2], digest[2:])
}
