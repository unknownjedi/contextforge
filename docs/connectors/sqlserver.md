# Microsoft SQL Server Connector Guide

## Overview
The SQL Server connector uses `github.com/microsoft/go-mssqldb` to introspect Microsoft SQL Server (2016 through 2022) and Azure SQL Database using system views (`sys.tables`, `sys.columns`, `sys.indexes`, `sys.foreign_keys`).

## URL Scheme
```
sqlserver://[user[:password]@][host][:1433][?database=dbname&param1=val1...]
mssql://[user[:password]@][host][:1433][?database=dbname&param1=val1...]
```

### Examples
- Standard TCP: `sqlserver://readonly_user:secret@mssql.internal:1433?database=erp_db&encrypt=true&TrustServerCertificate=false`
- Named instance: `sqlserver://readonly_user:secret@mssql.internal/SQLEXPRESS?database=erp_db`

## Introspected Objects
- Tables and views across all database schemas (e.g. `dbo`, `sales`).
- Columns, data types, lengths, nullability, and default constraints.
- Clustered and non-clustered primary keys and indexes.
- Foreign key relationships.

## Minimum Permissions
```sql
CREATE LOGIN contextforge_ro WITH PASSWORD = 'secure_password';
USE erp_db;
CREATE USER contextforge_ro FOR LOGIN contextforge_ro;
ALTER ROLE db_datareader ADD MEMBER contextforge_ro;
GRANT VIEW DEFINITION TO contextforge_ro;
```
