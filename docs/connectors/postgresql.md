# PostgreSQL Connector Guide

## Overview
The PostgreSQL connector utilizes `github.com/jackc/pgx/v5/stdlib` to introspect catalog schemas, foreign key topologies, indexes, and table definitions from PostgreSQL 12 through 17+.

## URL Scheme
```
postgresql://[user[:password]@][host][:port][/dbname][?param1=value1&...]
postgres://[user[:password]@][host][:port][/dbname][?param1=value1&...]
```

### Examples
- Standard connection: `postgresql://readonly_user:secret@db.example.com:5432/production_db?sslmode=require`
- Unix Domain Socket: `postgresql:///dbname?host=/var/run/postgresql`

## Introspected Objects
- Tables and views across all schemas or specified schemas (default: `public`).
- Columns, data types, nullability, default expressions, and comments.
- Primary key constraints and composite keys.
- Foreign keys with referenced schema, referenced table, and matching columns.
- Indexes (B-tree, Hash, GIN, GiST, BRIN) with uniqueness indicators.

## Minimum Permissions
```sql
CREATE USER contextforge_ro WITH PASSWORD 'secure_password';
GRANT CONNECT ON DATABASE my_db TO contextforge_ro;
GRANT USAGE ON SCHEMA public TO contextforge_ro;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO contextforge_ro;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO contextforge_ro;
```
