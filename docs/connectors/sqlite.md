# SQLite Connector Guide

## Overview
The SQLite connector uses `modernc.org/sqlite`, a 100% pure Go SQLite implementation that requires **zero CGO**. It works natively on all architectures without external C libraries or compilers.

## URL Scheme
```
sqlite:///[path/to/file.db][?mode=ro]
sqlite3:///[path/to/file.db]
```

### Examples
- Absolute path: `sqlite:///var/data/app.db`
- Read-only URI: `sqlite:///opt/sqlite/mydb.sqlite?mode=ro`

## Introspected Objects
- Tables and views from `sqlite_master`.
- Column definitions, data types, and nullability via `PRAGMA table_info`.
- Primary keys and autoincrement definitions.
- Foreign keys via `PRAGMA foreign_key_list`.
- Indexes via `PRAGMA index_list` and `PRAGMA index_info`.

## Security Notes
- SQLite operates on the local filesystem. In containerized environments, mount the target SQLite file or directory as a read-only volume.
