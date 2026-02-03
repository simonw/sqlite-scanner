package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckSQLiteMagic(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "good.db")
	content := append(append([]byte{}, sqliteMagic...), []byte("lengthy payload")...)
	if err := os.WriteFile(dbPath, content, 0o600); err != nil {
		t.Fatalf("write db: %v", err)
	}

	res, ok, err := checkSQLiteMagic(dbPath)
	if err != nil {
		t.Fatalf("checkSQLiteMagic: %v", err)
	}
	if !ok {
		t.Fatalf("expected header to match")
	}
	if res.Path != dbPath {
		t.Fatalf("path mismatch: %q vs %q", dbPath, res.Path)
	}
	if res.Size != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), res.Size)
	}

	badPath := filepath.Join(dir, "bad.bin")
	if err := os.WriteFile(badPath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("write bad file: %v", err)
	}

	if _, ok, err := checkSQLiteMagic(badPath); err != nil {
		t.Fatalf("check bad file: %v", err)
	} else if ok {
		t.Fatalf("expected bad header to be rejected")
	}
}

func TestFindSQLiteFilesMultipleRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()

	dbA := filepath.Join(rootA, "a.db")
	content := append(append([]byte{}, sqliteMagic...), []byte("foo")...)
	if err := os.WriteFile(dbA, content, 0o600); err != nil {
		t.Fatalf("write db A: %v", err)
	}

	if _, err := os.Create(filepath.Join(rootB, "empty.txt")); err != nil {
		t.Fatalf("create placeholder: %v", err)
	}

	results, err := findSQLiteFiles([]string{rootA, rootB}, runtime.NumCPU())
	if err != nil {
		t.Fatalf("findSQLiteFiles: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].Path != dbA {
		t.Fatalf("expected path %q, got %q", dbA, results[0].Path)
	}
	if results[0].Size != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), results[0].Size)
	}
}

func TestResolveRootsFollowsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	resolved := resolveRoots([]string{link})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 root, got %d", len(resolved))
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("eval symlink: %v", err)
	}
	if resolved[0] != want {
		t.Fatalf("expected %q, got %q", want, resolved[0])
	}
}

func TestJSONCommaPlacement(t *testing.T) {
	matches := make(chan matchResult, 2)
	matches <- matchResult{Path: "a.db", Size: 123}
	matches <- matchResult{Path: "b.db", Size: 456}
	close(matches)

	out := captureStdout(t, func() {
		streamMatches(matches, true, false, true)
	})
	if strings.Contains(out, "\n,\n") {
		t.Fatalf("got comma on its own line:\n%s", out)
	}
	if !strings.Contains(out, ",\n    {") {
		t.Fatalf("expected comma before entry, got:\n%s", out)
	}
}

func TestStreamMatchesPlainTextNoSize(t *testing.T) {
	matches := make(chan matchResult, 1)
	matches <- matchResult{Path: filepath.Join(t.TempDir(), "a.db"), Size: 123}
	close(matches)

	out := captureStdout(t, func() {
		streamMatches(matches, false, false, false)
	})
	if strings.Contains(out, "bytes") {
		t.Fatalf("expected no size in output, got: %s", out)
	}
	if !strings.Contains(out, "a.db") {
		t.Fatalf("expected path in output, got: %s", out)
	}
}

func TestStreamMatchesPlainTextWithSize(t *testing.T) {
	matches := make(chan matchResult, 1)
	matches <- matchResult{Path: filepath.Join(t.TempDir(), "b.db"), Size: 456}
	close(matches)

	out := captureStdout(t, func() {
		streamMatches(matches, false, false, true)
	})
	if !strings.Contains(out, "(456 bytes)") {
		t.Fatalf("expected size in output, got: %s", out)
	}
}

func TestStreamMatchesJSONNoSize(t *testing.T) {
	matches := make(chan matchResult, 1)
	matches <- matchResult{Path: filepath.Join(t.TempDir(), "c.db"), Size: 100}
	close(matches)

	out := captureStdout(t, func() {
		streamMatches(matches, true, false, false)
	})
	if strings.Contains(out, "\"size\"") {
		t.Fatalf("expected no size field, got: %s", out)
	}
	if !strings.Contains(out, "\"path\"") {
		t.Fatalf("expected JSON entries with path, got: %s", out)
	}
	if !strings.Contains(out, "\"entries\"") {
		t.Fatalf("expected entries key, got: %s", out)
	}
}

func TestStreamMatchesJSONWithSize(t *testing.T) {
	matches := make(chan matchResult, 1)
	matches <- matchResult{Path: filepath.Join(t.TempDir(), "d.db"), Size: 222}
	close(matches)

	out := captureStdout(t, func() {
		streamMatches(matches, true, false, true)
	})
	if !strings.Contains(out, "\"size\": 222") {
		t.Fatalf("expected size field, got: %s", out)
	}
	if !strings.Contains(out, "\"entries\"") {
		t.Fatalf("expected entries key, got: %s", out)
	}
}

func TestStreamMatchesJSONLNoSize(t *testing.T) {
	matches := make(chan matchResult, 2)
	matches <- matchResult{Path: filepath.Join(t.TempDir(), "e.db"), Size: 11}
	matches <- matchResult{Path: filepath.Join(t.TempDir(), "f.db"), Size: 22}
	close(matches)

	out := captureStdout(t, func() {
		streamMatches(matches, false, true, false)
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d: %s", len(lines), out)
	}
	for _, line := range lines {
		if strings.Contains(line, "\"size\"") {
			t.Fatalf("expected no size field, got: %s", line)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("invalid JSON line: %v", err)
		}
		if _, ok := obj["path"]; !ok {
			t.Fatalf("expected path field, got: %v", obj)
		}
	}
}

func TestStreamMatchesJSONLWithSize(t *testing.T) {
	matches := make(chan matchResult, 1)
	matches <- matchResult{Path: filepath.Join(t.TempDir(), "g.db"), Size: 33}
	close(matches)

	out := captureStdout(t, func() {
		streamMatches(matches, false, true, true)
	})
	line := strings.TrimSpace(out)
	if !strings.Contains(line, "\"size\"") {
		t.Fatalf("expected size field in JSONL, got: %s", line)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("invalid JSON line: %v", err)
	}
	if _, ok := obj["size"]; !ok {
		t.Fatalf("expected size field, got: %v", obj)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()
	w.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy: %v", err)
	}
	r.Close()

	return buf.String()
}
