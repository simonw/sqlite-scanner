# sqlite-scanner

`sqlite-scanner` is a tiny Go CLI that recurses through one or more directories, checks each regular file’s header bytes, and reports the ones whose first 16 bytes match `SQLite format 3\x00`. It never trusts file extensions, and it can run multiple workers in parallel for speed.

## Features

- scans one or more positional paths or falls back to `.` when no paths are specified
- configurable worker pool via `--workers` (defaults to your CPU count)
- always prints absolute paths so results are unambiguous
- optional `--size` flag that appends file sizes to text output and emits JSON objects like `{"path": "...", "size": ...}`
- newline-delimited JSON via `--jsonl`; use `--size` to include each object's size field
- JSON output mode (`--json`) that pretty-prints `{"entries": [...]}` objects for downstream processing
- streams matches immediately as they’re discovered (plain text and pretty JSON)
- custom `--help` text that describes usage, examples, and notes

## Build

```bash
cd ~/dev/sqlite-scanner
go build -o bin/sqlite-scanner
```

That creates `bin/sqlite-scanner`, the same binary you already used earlier.

## Usage

Simple scan (current directory):

```bash
~/dev/sqlite-scanner/bin/sqlite-scanner
```

Scan `/tmp` and `$HOME`:

```bash
~/dev/sqlite-scanner/bin/sqlite-scanner /tmp ~
```

Use JSON mode:

```bash
~/dev/sqlite-scanner/bin/sqlite-scanner /tmp --json

Example JSON output shape:

```json
{
  "entries": [
    {"path": "/abs/path/to/db1.sqlite"},
    {"path": "/abs/path/to/db2.sqlite", "size": 12345}
  ]
}
```
```

Use newline-delimited JSON to stream objects per line (requires `--size` to include size):

```bash
~/dev/sqlite-scanner/bin/sqlite-scanner --jsonl ~/dev
```

Include sizes (plain text shows `(size bytes)` and JSON outputs objects) with:

```bash
~/dev/sqlite-scanner/bin/sqlite-scanner --size /tmp --json
```

Check available flags (it prints the detailed help text added earlier; all flags use the `--flag` form):

```bash
~/dev/sqlite-scanner/bin/sqlite-scanner --help
```

## Testing

```bash
go test
```
