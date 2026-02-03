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

## Setup

```bash
git clone https://github.com/simonw/sqlite-scanner
cd sqlite-scanner
```

## Build

```bash
go build -o bin/sqlite-scanner
```

## Usage

Simple scan (current directory):

```bash
bin/sqlite-scanner
```

Scan `/tmp` and `$HOME`:

```bash
bin/sqlite-scanner /tmp ~
```

Use JSON mode:

```bash
bin/sqlite-scanner /tmp --json
```

Example JSON output shape:

```json
{
  "entries": [
    {"path": "/abs/path/to/db1.sqlite"},
    {"path": "/abs/path/to/db2.sqlite"}
  ]
}
```

Use newline-delimited JSON to stream objects per line (requires `--size` to include size):

```bash
bin/sqlite-scanner --jsonl ~/dev
```

Example JSONL output shape (no size):

```jsonl
{"path": "/abs/path/to/db1.sqlite"}
{"path": "/abs/path/to/db2.sqlite"}
```

Example JSONL output shape (with `--size`):

```jsonl
{"path": "/abs/path/to/db1.sqlite", "size": 12345}
{"path": "/abs/path/to/db2.sqlite", "size": 67890}
```

Include sizes (plain text shows `(size bytes)` and JSON outputs objects) with:

```bash
bin/sqlite-scanner --size /tmp --json
```

Example `--size` output (plain text):

```
/abs/path/to/db1.sqlite (12345 bytes)
/abs/path/to/db2.sqlite (67890 bytes)
```

Check available flags (it prints the detailed help text added earlier; all flags use the `--flag` form):

```bash
bin/sqlite-scanner --help
```

## Testing

```bash
go test
```
