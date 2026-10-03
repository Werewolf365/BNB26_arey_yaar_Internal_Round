// Package storage is the content-addressed evidence blob backend.
// Layout: <root>/sha256/<hex[0:2]>/<hex[2:]> (git-style fanout).
// Keys are ALWAYS derived from content hashes server-side; user-supplied
// paths are never trusted (traversal, symlink, and size guards enforced).
// Filesystem is the first backend; the S3-compatible backend implements the
// same Backend interface (see docs for the LocalStack-tested rollout).
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Backend stores opaque blobs under content keys.
type Backend interface {
	// Put stores data, returning its content key (sha256/<hex>).
	Put(data []byte) (string, error)
	// Get retrieves a blob by content key. Unknown keys are NOT_FOUND errors.
	Get(key string) ([]byte, error)
	// Stat reports existence without reading the blob.
	Stat(key string) (bool, error)
}

// Limits.
const (
	MaxBlobBytes = 64 * 1024 * 1024
)

// Filesystem backend rooted at dir.
type Filesystem struct {
	root string
}

// NewFilesystem creates (if needed) and validates the root.
func NewFilesystem(root string) (*Filesystem, error) {
	if root == "" {
		return nil, fmt.Errorf("INVALID_INPUT: storage root required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("OPERATIONAL: storage root: %v", err)
	}
	return &Filesystem{root: abs}, nil
}

// validateContentKey enforces canonical sha256/<64 lowercase hex> shape.
// Shared by the Filesystem and S3 backends; returns the hex digest.
func validateContentKey(key string) (string, error) {
	parts := strings.Split(key, "/")
	if len(parts) != 2 || parts[0] != "sha256" || len(parts[1]) != 64 {
		return "", fmt.Errorf("INVALID_INPUT: bad content key %q", key)
	}
	for _, c := range parts[1] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", fmt.Errorf("INVALID_INPUT: bad content key %q", key)
		}
	}
	return parts[1], nil
}

// keyPath validates a content key and resolves it under the root.
// Only canonical sha256/<64hex> keys are accepted — nothing else.
func (f *Filesystem) keyPath(key string) (string, error) {
	hexpart, err := validateContentKey(key)
	if err != nil {
		return "", err
	}
	p := filepath.Join(f.root, "sha256", hexpart[:2], hexpart[2:])
	// Belt-and-braces: resolved path must stay under root (symlink-safe:
	// writers use O_EXCL create + rename; readers resolve then re-check).
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if abs != f.root && !strings.HasPrefix(abs, f.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("INVALID_INPUT: key escapes storage root")
	}
	return abs, nil
}

// Put writes atomically (temp file + rename) and verifies the hash.
func (f *Filesystem) Put(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("INVALID_INPUT: empty blob")
	}
	if len(data) > MaxBlobBytes {
		return "", fmt.Errorf("ARTIFACT_OVERSIZED: %d bytes exceeds %d", len(data), MaxBlobBytes)
	}
	sum := sha256.Sum256(data)
	key := "sha256/" + hex.EncodeToString(sum[:])
	p, err := f.keyPath(key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err == nil {
		return key, nil // already stored: content-addressed dedup
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup; rename success makes remove a no-op error.
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, p); err != nil {
		// Lost race with a concurrent writer (Windows also refuses rename
		// onto an existing file): the winner's bytes must verify.
		for i := 0; i < 40; i++ {
			got, rerr := f.Get(key)
			if rerr == nil {
				if string(got) != string(data) {
					return "", fmt.Errorf("INTERNAL_ERROR: content mismatch at %s", key)
				}
				return key, nil
			}
			time.Sleep(50 * time.Millisecond)
		}
		return "", fmt.Errorf("OPERATIONAL: concurrent store race: %v", err)
	}
	return key, nil
}

// Get reads a blob and re-verifies its hash (tamper-evident reads).
func (f *Filesystem) Get(key string) ([]byte, error) {
	p, err := f.keyPath(key)
	if err != nil {
		return nil, err
	}
	fh, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("EVIDENCE_NOT_FOUND: %s", key)
		}
		return nil, err
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxBlobBytes {
		return nil, fmt.Errorf("ARTIFACT_OVERSIZED: stored blob exceeds cap")
	}
	data, err := io.ReadAll(io.LimitReader(fh, MaxBlobBytes+1))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	want := strings.TrimPrefix(key, "sha256/")
	if hex.EncodeToString(sum[:]) != want {
		return nil, fmt.Errorf("AUDIT_TAMPERED: stored blob %s fails hash check", key)
	}
	return data, nil
}

// Stat reports existence.
func (f *Filesystem) Stat(key string) (bool, error) {
	p, err := f.keyPath(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
