# ABAP SQL Gotchas for RunQuery

## Critical Syntax Differences

ABAP SQL is NOT standard SQL. These are the most common mistakes:

### ORDER BY

```sql
-- WRONG (standard SQL)
SELECT * FROM MARA ORDER BY MATNR DESC

-- CORRECT (ABAP SQL)
SELECT * FROM MARA ORDER BY MATNR DESCENDING
```

Keywords: `ASCENDING` (default), `DESCENDING` — no abbreviations.

### LIMIT / Row Restriction

```sql
-- WRONG (no LIMIT in ABAP SQL)
SELECT * FROM MARA LIMIT 10

-- CORRECT (use max_rows parameter)
RunQuery(query="SELECT * FROM MARA", max_rows=10)
```

The `max_rows` parameter is passed to the tool, not embedded in the SQL.

### Aggregate Functions

```sql
-- These work in ABAP SQL
SELECT COUNT(*) FROM MARA WHERE MTART = 'FERT'
SELECT MTART, COUNT(*) AS cnt FROM MARA GROUP BY MTART
SELECT MAX(ERDAT) FROM MARA
```

### String Literals

```sql
-- Use single quotes
SELECT * FROM MARA WHERE MTART = 'FERT'

-- Wildcards use % (like standard SQL)
SELECT * FROM MARA WHERE MATNR LIKE 'MAT%'
```

### Table Joins

```sql
-- Inner join
SELECT a~MATNR, b~MAKTX
  FROM MARA AS a
  INNER JOIN MAKT AS b ON a~MATNR = b~MATNR
  WHERE b~SPRAS = 'E'
```

### Common Pitfalls

1. **No `DESC`** — always spell out `DESCENDING`
2. **No `LIMIT`** — use `max_rows` parameter
3. **No `TOP n`** — use `max_rows` parameter
4. **Field names are uppercase** — `MATNR` not `matnr`
5. **Client handling** — system auto-filters by client; don't add `WHERE MANDT = ...`
6. **`BlockFreeSQL` safety** — RunQuery may be blocked by `--block-free-sql` safety config
