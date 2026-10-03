package storage_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/quorum/quorum/services/storage"
)

func TestPutGetRoundTrip(t *testing.T) {
	fs, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, err := fs.Put([]byte("evidence-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "sha256/") {
		t.Fatalf("content key: %q", key)
	}
	got, err := fs.Get(key)
	if err != nil || string(got) != "evidence-bytes" {
		t.Fatalf("round-trip: %v", err)
	}
	// Dedup: same content, same key.
	key2, err := fs.Put([]byte("evidence-bytes"))
	if err != nil || key2 != key {
		t.Fatal("dedup")
	}
	ok, err := fs.Stat(key)
	if err != nil || !ok {
		t.Fatal("stat")
	}
	if ok, _ := fs.Stat("sha256/" + strings.Repeat("0", 64)); ok {
		t.Fatal("missing must stat false")
	}
}

func TestTraversalAndShapeRejected(t *testing.T) {
	fs, _ := storage.NewFilesystem(t.TempDir())
	for _, bad := range []string{
		"", "../../etc/passwd", "sha256/../../x", "sha256/xyz",
		"sha256/" + strings.Repeat("g", 64), "md5/abc", "sha256/abc/extra",
		"sha256/" + strings.Repeat("A", 64), // uppercase rejected (canonical lowercase)
	} {
		if _, err := fs.Get(bad); err == nil {
			t.Fatalf("Get(%q) must fail", bad)
		}
		if _, err := fs.Stat(bad); err == nil {
			t.Fatalf("Stat(%q) must fail", bad)
		}
	}
	if _, err := fs.Put(nil); err == nil {
		t.Fatal("empty blob must fail")
	}
	if _, err := storage.NewFilesystem(""); err == nil {
		t.Fatal("empty root must fail")
	}
}

func TestOnDiskTamperDetected(t *testing.T) {
	dir := t.TempDir()
	fs, _ := storage.NewFilesystem(dir)
	key, _ := fs.Put([]byte("pristine"))
	// Find the file and corrupt it (simulates disk tampering).
	var target string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			target = p
		}
		return nil
	})
	os.WriteFile(target, []byte("corrupted!!"), 0o600)
	if _, err := fs.Get(key); err == nil || !strings.Contains(err.Error(), "AUDIT_TAMPERED") {
		t.Fatalf("tampered blob must fail hash check: %v", err)
	}
}

func TestConcurrentPut(t *testing.T) {
	fs, _ := storage.NewFilesystem(t.TempDir())
	var wg sync.WaitGroup
	errs := make([]error, 8)
	keys := make([]string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			keys[i], errs[i] = fs.Put([]byte("same-content"))
		}(i)
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil || keys[i] != keys[0] {
			t.Fatalf("concurrent put: %v %q", errs[i], keys[i])
		}
	}
}
