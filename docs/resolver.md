# Query Resolution

## Overview

The Resolver translates GraphQL queries into SQL statements and executes them against PostgreSQL. It handles queries, mutations, and aggregations.

## Resolver Architecture

```
GraphQL Query
      │
      ▼
┌─────────────────────────────────────────────────────────┐
│                    Resolver                              │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────┐                                       │
│  │ Query Parser │ ──► Extract table, columns, filters   │
│  └──────┬───────┘                                       │
│         │                                                │
│         ▼                                                │
│  ┌──────────────┐                                       │
│  │ SQL Builder  │ ──► Construct parameterized SQL       │
│  └──────┬───────┘                                       │
│         │                                                │
│         ▼                                                │
│  ┌──────────────┐                                       │
│  │ Executor     │ ──► Run against pgxpool               │
│  └──────┬───────┘                                       │
│         │                                                │
│         ▼                                                │
│  ┌──────────────┐                                       │
│  │ Row Scanner  │ ──► Map results to Go types           │
│  └──────────────┘                                       │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Query Parameters

```go
// internal/resolver/resolver.go

type QueryParams struct {
    TableName   string                   // Target table
    Columns     []string                 // Columns to select
    Where       map[string]interface{}   // Filter conditions
    OrderBy     []OrderByClause          // Sorting
    Limit       *int                     // Result limit
    Offset      *int                     // Pagination offset
    Distinct    bool                     // DISTINCT results
}

type OrderByClause struct {
    Column string    // Column name
    Order  string    // "asc" or "desc"
    Nulls  string    // "first" or "last"
}
```

## Query Resolution Flow

### 1. Simple Query

**GraphQL:**
```graphql
query {
  users(where: {age: {_gt: 18}}, limit: 10) {
    id
    name
    email
  }
}
```

**Generated SQL:**
```sql
SELECT id, name, email
FROM public.users
WHERE age > $1
LIMIT 10
-- Parameters: [18]
```

### 2. Query with Ordering

**GraphQL:**
```graphql
query {
  users(order_by: {created_at: desc, name: asc}) {
    id
    name
    created_at
  }
}
```

**Generated SQL:**
```sql
SELECT id, name, created_at
FROM public.users
ORDER BY created_at DESC, name ASC
```

### 3. Query with Nested Filtering

**GraphQL:**
```graphql
query {
  users(where: {
    _and: [
      {age: {_gte: 18}},
      {_or: [
        {status: {_eq: "active"}},
        {role: {_eq: "admin"}}
      ]}
    ]
  }) {
    id
    name
  }
}
```

**Generated SQL:**
```sql
SELECT id, name
FROM public.users
WHERE (age >= $1 AND (status = $2 OR role = $3))
-- Parameters: [18, "active", "admin"]
```

## SQL Builder Implementation

```go
// internal/resolver/resolver.go

func (r *Resolver) buildSelectQuery(params QueryParams) (string, []interface{}) {
    var args []interface{}
    argIndex := 1

    // SELECT clause
    columns := "*"
    if len(params.Columns) > 0 {
        columns = strings.Join(params.Columns, ", ")
    }

    query := fmt.Sprintf("SELECT %s FROM %s.%s",
        columns, r.defaultSchema, params.TableName)

    // WHERE clause
    if len(params.Where) > 0 {
        whereClause, whereArgs := r.buildWhereClause(params.Where, &argIndex)
        query += " WHERE " + whereClause
        args = append(args, whereArgs...)
    }

    // ORDER BY clause
    if len(params.OrderBy) > 0 {
        orderClauses := make([]string, len(params.OrderBy))
        for i, ob := range params.OrderBy {
            orderClauses[i] = fmt.Sprintf("%s %s", ob.Column, strings.ToUpper(ob.Order))
            if ob.Nulls != "" {
                orderClauses[i] += fmt.Sprintf(" NULLS %s", strings.ToUpper(ob.Nulls))
            }
        }
        query += " ORDER BY " + strings.Join(orderClauses, ", ")
    }

    // LIMIT/OFFSET
    if params.Limit != nil {
        query += fmt.Sprintf(" LIMIT %d", *params.Limit)
    }
    if params.Offset != nil {
        query += fmt.Sprintf(" OFFSET %d", *params.Offset)
    }

    return query, args
}
```

## Filter Operators

| Operator | Description | SQL Equivalent |
|----------|-------------|----------------|
| `_eq` | Equal | `= $1` |
| `_neq` | Not equal | `!= $1` |
| `_gt` | Greater than | `> $1` |
| `_gte` | Greater or equal | `>= $1` |
| `_lt` | Less than | `< $1` |
| `_lte` | Less or equal | `<= $1` |
| `_in` | In array | `IN ($1, $2, ...)` |
| `_nin` | Not in array | `NOT IN ($1, $2, ...)` |
| `_is_null` | Is null | `IS NULL` / `IS NOT NULL` |
| `_like` | Pattern match | `LIKE $1` |
| `_ilike` | Case-insensitive pattern | `ILIKE $1` |
| `_similar` | Similar to | `SIMILAR TO $1` |
| `_regex` | Regular expression | `~ $1` |
| `_iregex` | Case-insensitive regex | `~* $1` |

### Filter Implementation

```go
func (r *Resolver) buildWhereClause(where map[string]interface{}, argIndex *int) (string, []interface{}) {
    var conditions []string
    var args []interface{}

    for key, value := range where {
        switch key {
        case "_and":
            // Handle AND condition
            subConditions := value.([]interface{})
            var andClauses []string
            for _, sub := range subConditions {
                clause, subArgs := r.buildWhereClause(sub.(map[string]interface{}), argIndex)
                andClauses = append(andClauses, clause)
                args = append(args, subArgs...)
            }
            conditions = append(conditions, "("+strings.Join(andClauses, " AND ")+")")

        case "_or":
            // Handle OR condition
            subConditions := value.([]interface{})
            var orClauses []string
            for _, sub := range subConditions {
                clause, subArgs := r.buildWhereClause(sub.(map[string]interface{}), argIndex)
                orClauses = append(orClauses, clause)
                args = append(args, subArgs...)
            }
            conditions = append(conditions, "("+strings.Join(orClauses, " OR ")+")")

        case "_not":
            // Handle NOT condition
            clause, subArgs := r.buildWhereClause(value.(map[string]interface{}), argIndex)
            conditions = append(conditions, "NOT ("+clause+")")
            args = append(args, subArgs...)

        default:
            // Column filter
            colCondition, colArgs := r.buildColumnCondition(key, value.(map[string]interface{}), argIndex)
            conditions = append(conditions, colCondition)
            args = append(args, colArgs...)
        }
    }

    return strings.Join(conditions, " AND "), args
}

func (r *Resolver) buildColumnCondition(column string, ops map[string]interface{}, argIndex *int) (string, []interface{}) {
    var conditions []string
    var args []interface{}

    for op, val := range ops {
        switch op {
        case "_eq":
            conditions = append(conditions, fmt.Sprintf("%s = $%d", column, *argIndex))
            args = append(args, val)
            (*argIndex)++
        case "_neq":
            conditions = append(conditions, fmt.Sprintf("%s != $%d", column, *argIndex))
            args = append(args, val)
            (*argIndex)++
        case "_gt":
            conditions = append(conditions, fmt.Sprintf("%s > $%d", column, *argIndex))
            args = append(args, val)
            (*argIndex)++
        // ... other operators
        case "_is_null":
            if val.(bool) {
                conditions = append(conditions, fmt.Sprintf("%s IS NULL", column))
            } else {
                conditions = append(conditions, fmt.Sprintf("%s IS NOT NULL", column))
            }
        case "_in":
            placeholders := make([]string, len(val.([]interface{})))
            for i, v := range val.([]interface{}) {
                placeholders[i] = fmt.Sprintf("$%d", *argIndex)
                args = append(args, v)
                (*argIndex)++
            }
            conditions = append(conditions, fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ", ")))
        }
    }

    return strings.Join(conditions, " AND "), args
}
```

## Mutations

### Insert

**GraphQL:**
```graphql
mutation {
  insert_users(objects: [{name: "John", email: "john@example.com"}]) {
    affected_rows
    returning {
      id
      name
    }
  }
}
```

**Generated SQL:**
```sql
INSERT INTO public.users (name, email)
VALUES ($1, $2)
RETURNING id, name
-- Parameters: ["John", "john@example.com"]
```

### Update

**GraphQL:**
```graphql
mutation {
  update_users(where: {id: {_eq: 1}}, _set: {name: "Jane"}) {
    affected_rows
    returning {
      id
      name
    }
  }
}
```

**Generated SQL:**
```sql
UPDATE public.users
SET name = $1
WHERE id = $2
RETURNING id, name
-- Parameters: ["Jane", 1]
```

### Delete

**GraphQL:**
```graphql
mutation {
  delete_users(where: {id: {_eq: 1}}) {
    affected_rows
  }
}
```

**Generated SQL:**
```sql
DELETE FROM public.users
WHERE id = $1
RETURNING *
-- Parameters: [1]
```

## Aggregations

### Aggregate Query

**GraphQL:**
```graphql
query {
  users_aggregate(where: {status: {_eq: "active"}}) {
    aggregate {
      count
      sum {
        age
      }
      avg {
        age
      }
      max {
        created_at
      }
      min {
        created_at
      }
    }
  }
}
```

**Generated SQL:**
```sql
SELECT
    COUNT(*) as count,
    SUM(age) as sum_age,
    AVG(age) as avg_age,
    MAX(created_at) as max_created_at,
    MIN(created_at) as min_created_at
FROM public.users
WHERE status = $1
-- Parameters: ["active"]
```

## Row Scanning

```go
// internal/resolver/resolver.go

func (r *Resolver) scanRows(rows pgx.Rows) ([]map[string]interface{}, error) {
    var results []map[string]interface{}

    // Get column descriptions
    fieldDescs := rows.FieldDescriptions()
    columns := make([]string, len(fieldDescs))
    for i, fd := range fieldDescs {
        columns[i] = string(fd.Name)
    }

    for rows.Next() {
        // Get row values as []interface{}
        values, err := rows.Values()
        if err != nil {
            return nil, err
        }

        // Map to column names
        row := make(map[string]interface{})
        for i, col := range columns {
            row[col] = values[i]
        }
        results = append(results, row)
    }

    if err := rows.Err(); err != nil {
        return nil, err
    }

    return results, nil
}
```

## Type Mapping

| PostgreSQL Type | Go Type | GraphQL Type |
|-----------------|---------|--------------|
| integer, serial | int32 | Int |
| bigint, bigserial | int64 | Int |
| smallint | int16 | Int |
| real | float32 | Float |
| double precision | float64 | Float |
| numeric, decimal | pgtype.Numeric | Float |
| boolean | bool | Boolean |
| text, varchar, char | string | String |
| uuid | pgtype.UUID | String |
| timestamp, timestamptz | time.Time | String (ISO8601) |
| date | time.Time | String |
| json, jsonb | map[string]interface{} | JSON |
| array types | []T | [Type] |

## Performance Optimizations

1. **Parameterized Queries**: Prevent SQL injection, enable query plan caching
2. **Column Selection**: Only fetch requested columns
3. **Connection Reuse**: Use pgxpool for connection pooling
4. **Batch Operations**: Use SendBatch for related queries
5. **Prepared Statements**: Cache frequently used queries

## Error Handling

```go
func (r *Resolver) ResolveQuery(ctx context.Context, params QueryParams) ([]map[string]interface{}, error) {
    sql, args := r.buildSelectQuery(params)

    rows, err := r.db.Query(ctx, sql, args...)
    if err != nil {
        // Check for specific PostgreSQL errors
        var pgErr *pgconn.PgError
        if errors.As(err, &pgErr) {
            switch pgErr.Code {
            case "42P01": // undefined_table
                return nil, fmt.Errorf("table '%s' does not exist", params.TableName)
            case "42703": // undefined_column
                return nil, fmt.Errorf("column does not exist: %s", pgErr.Message)
            case "22P02": // invalid_text_representation
                return nil, fmt.Errorf("invalid input value: %s", pgErr.Message)
            }
        }
        return nil, fmt.Errorf("query execution failed: %w", err)
    }
    defer rows.Close()

    return r.scanRows(rows)
}
```
