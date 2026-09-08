# CockroachDB Connector Guide

## Overview
CockroachDB uses the PostgreSQL wire protocol and is fully supported via the standard PostgreSQL connector engine with CockroachDB-specific dialect adaptations.

## URL Scheme
```
postgresql://[user[:password]@][host][:26257][/dbname][?sslmode=verify-full&sslrootcert=...]
cockroachdb://[user[:password]@][host][:26257][/dbname]
```

## Introspected Objects
- Distributed relational tables and views.
- Interleaved tables and secondary indexes.
- Columns, computed columns, types, nullability, and primary/foreign keys.

## Minimum Permissions
```sql
CREATE USER contextforge_ro WITH PASSWORD 'secure_password';
GRANT SELECT ON DATABASE my_db TO contextforge_ro;
```
