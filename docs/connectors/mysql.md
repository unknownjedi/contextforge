# MySQL & MariaDB Connector Guide

## Overview
The MySQL connector uses `github.com/go-sql-driver/mysql` to introspect MySQL (5.7, 8.0, 8.4 LTS) and MariaDB (10.4+) servers via `information_schema` catalogs.

## URL Scheme
```
mysql://[user[:password]@][host][:port][/dbname][?param1=value1&...]
mariadb://[user[:password]@][host][:port][/dbname][?param1=value1&...]
```

### Examples
- Standard connection: `mysql://readonly_user:secret@mysql.internal:3306/ecommerce_db?parseTime=true`
- SSL connection: `mysql://readonly_user:secret@mysql.internal:3306/ecommerce_db?tls=true`

## Introspected Objects
- Tables and views in `information_schema.tables`.
- Columns, collation, types, nullability, defaults, and column comments.
- Primary key constraints (`information_schema.table_constraints`).
- Foreign keys (`information_schema.key_column_usage`).
- Indexes (`information_schema.statistics`).

## Minimum Permissions
```sql
CREATE USER 'contextforge_ro'@'%' IDENTIFIED BY 'secure_password';
GRANT SELECT, SHOW VIEW ON ecommerce_db.* TO 'contextforge_ro'@'%';
FLUSH PRIVILEGES;
```
