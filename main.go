package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/spf13/pflag"
)

var sqliteMagic = []byte("SQLite format 3\x00")
var version = "dev"

type matchResult struct {
	Path string
	Size int64
}

func main() {
	root := pflag.String("path", ".", "directory to scan")
	workers := pflag.Int("workers", runtime.NumCPU(), "number of parallel workers")
	jsonOutput := pflag.Bool("json", false, "print matches as a JSON object with an entries array")
	size := pflag.Bool("size", false, "include the file size (bytes) in the output")
	jsonl := pflag.Bool("jsonl", false, "emit newline-delimited JSON objects")
	versionFlag := pflag.Bool("version", false, "print version and exit")

	pflag.Usage = func() {
		out := os.Stdout
		fmt.Fprintln(out, "sqlite-scanner")
		fmt.Fprintln(out, "  Recursively find SQLite database files by checking file magic bytes.")
		fmt.Fprintln(out, "  Detection does not rely on file extensions and accepts positional paths.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Usage:")
		fmt.Fprintln(out, "  sqlite-scanner [flags] [paths...] (flags accept --flag form anywhere)")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Flags (use --flag form):")
		pflag.PrintDefaults()
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Examples:")
		fmt.Fprintln(out, "  sqlite-scanner")
		fmt.Fprintln(out, "  sqlite-scanner /tmp")
		fmt.Fprintln(out, "  sqlite-scanner /tmp ~")
		fmt.Fprintln(out, "  sqlite-scanner --workers 16 /tmp")
		fmt.Fprintln(out, "  sqlite-scanner --json")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Notes:")
		fmt.Fprintln(out, "  - Matches files with header bytes: \"SQLite format 3\\x00\".")
		fmt.Fprintln(out, "  - Permission-denied paths are skipped.")
		fmt.Fprintln(out, "  - Worker pool is controlled by `--workers`.")
		fmt.Fprintln(out, "  - Output is streamed as entries are discovered.")
		fmt.Fprintln(out, "  - Use --jsonl (with --size) to emit newline-delimited JSON objects.")
	}

	pflag.Parse()

	if *versionFlag {
		fmt.Println(version)
		return
	}

	positions := pflag.Args()
	roots := positions
	if len(roots) == 0 {
		roots = []string{*root}
	}
	roots = resolveRoots(roots)

	if *workers <= 0 {
		fmt.Fprintln(os.Stderr, "workers must be > 0")
		os.Exit(2)
	}
	matches := make(chan matchResult, *workers*2)
	errs := make(chan error, *workers)

	var printWg sync.WaitGroup
	printWg.Add(1)
	go func() {
		defer printWg.Done()
		showSize := *size
		streamMatches(matches, *jsonOutput, *jsonl, showSize)
	}()

	var warnWg sync.WaitGroup
	warnWg.Add(1)
	go func() {
		defer warnWg.Done()
		for err := range errs {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}
	}()

	walkErr := scanPaths(roots, *workers, matches, errs)

	printWg.Wait()
	warnWg.Wait()

	if walkErr != nil {
		fmt.Fprintf(os.Stderr, "scan completed with walk error: %v\n", walkErr)
	}
}

func findSQLiteFiles(roots []string, workers int) ([]matchResult, error) {
	matches := make(chan matchResult, workers*2)
	errs := make(chan error, workers)

	var out []matchResult
	var collectWg sync.WaitGroup
	collectWg.Add(1)
	go func() {
		defer collectWg.Done()
		for m := range matches {
			out = append(out, m)
		}
	}()

	var drainWg sync.WaitGroup
	drainWg.Add(1)
	go func() {
		defer drainWg.Done()
		for range errs {
		}
	}()

	walkErr := scanPaths(roots, workers, matches, errs)
	collectWg.Wait()
	drainWg.Wait()

	return out, walkErr
}

func streamMatches(matches <-chan matchResult, jsonOutput bool, jsonl bool, showSize bool) {
	if jsonl {
		for m := range matches {
			fmt.Println(formatJSONLine(m, showSize))
		}
		return
	}

	if jsonOutput {
		fmt.Println("{")
		fmt.Println("  \"entries\": [")
		first, ok := <-matches
		if ok {
			curr := first
			for next := range matches {
				entry := formatJSONEntry(curr, showSize)
				fmt.Printf("%s,\n", entry)
				curr = next
			}
			fmt.Println(formatJSONEntry(curr, showSize))
		}
		fmt.Println("  ]")
		fmt.Println("}")
		return
	}

	for m := range matches {
		fmt.Println(formatPlainMatch(m, showSize))
	}
}

func formatPath(path string) string {
	if ap, err := filepath.Abs(path); err == nil {
		return ap
	}
	return path
}

func formatJSONLine(m matchResult, showSize bool) string {
	path := formatPath(m.Path)
	if showSize {
		return fmt.Sprintf("{\"path\": %s, \"size\": %d}", marshalString(path), m.Size)
	}
	return fmt.Sprintf("{\"path\": %s}", marshalString(path))
}

func formatJSONEntry(m matchResult, showSize bool) string {
	path := formatPath(m.Path)
	if showSize {
		return fmt.Sprintf("    {\n      \"path\": %s,\n      \"size\": %d\n    }", marshalString(path), m.Size)
	}
	return fmt.Sprintf("    {\n      \"path\": %s\n    }", marshalString(path))
}

func formatPlainMatch(m matchResult, showSize bool) string {
	path := formatPath(m.Path)
	if !showSize {
		return path
	}
	return fmt.Sprintf("%s (%d bytes)", path, m.Size)
}

func marshalString(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func resolveRoots(roots []string) []string {
	resolved := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		r := root
		if resolvedRoot, err := filepath.EvalSymlinks(root); err == nil {
			r = resolvedRoot
		}
		if _, err := os.Stat(r); err == nil {
			if _, ok := seen[r]; ok {
				continue
			}
			seen[r] = struct{}{}
			resolved = append(resolved, r)
			continue
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		resolved = append(resolved, root)
	}
	return resolved
}

func scanPaths(roots []string, workers int, matches chan<- matchResult, errs chan<- error) error {
	paths := make(chan string, workers*4)

	var workerWg sync.WaitGroup
	for i := 0; i < workers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for p := range paths {
				res, ok, err := checkSQLiteMagic(p)
				if err != nil {
					if !errors.Is(err, fs.ErrPermission) {
						errs <- fmt.Errorf("%s: %w", p, err)
					}
					continue
				}
				if ok {
					matches <- res
				}
			}
		}()
	}

	var walkErr error
	var walkErrMu sync.Mutex
	var walkWg sync.WaitGroup

	for _, root := range roots {
		walkWg.Add(1)
		go func(r string) {
			defer walkWg.Done()
			err := filepath.WalkDir(r, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					if errors.Is(err, fs.ErrPermission) {
						return nil
					}
					return err
				}
				if d.Type().IsRegular() {
					paths <- path
				}
				return nil
			})
			if err != nil {
				walkErrMu.Lock()
				walkErr = errors.Join(walkErr, err)
				walkErrMu.Unlock()
			}
		}(root)
	}

	go func() {
		walkWg.Wait()
		close(paths)
	}()

	workerWg.Wait()
	close(matches)
	close(errs)

	return walkErr
}

func checkSQLiteMagic(path string) (matchResult, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return matchResult{}, false, err
	}
	defer f.Close()

	buf := make([]byte, len(sqliteMagic))
	if _, err := io.ReadFull(f, buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return matchResult{}, false, nil
		}
		return matchResult{}, false, err
	}

	if !bytes.Equal(buf, sqliteMagic) {
		return matchResult{}, false, nil
	}

	info, err := f.Stat()
	if err != nil {
		return matchResult{}, false, err
	}

	return matchResult{
		Path: path,
		Size: info.Size(),
	}, true, nil
}
