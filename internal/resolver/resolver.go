package resolver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/graphpost/graphpost/internal/database"
)

// Resolver handles GraphQL query resolution
type Resolver struct {
	db       *sql.DB
	schema   *database.Schema
	dbSchema string
}

// NewResolver creates a new resolver
func NewResolver(db *sql.DB, schema *database.Schema, dbSchema string) *Resolver {
	if dbSchema == "" {
		dbSchema = "public"
	}
	return &Resolver{
		db:       db,
		schema:   schema,
		dbSchema: dbSchema,
	}
}

// QueryParams represents parameters for a query
type QueryParams struct {
	TableName  string
	Where      map[string]interface{}
	OrderBy    []map[string]interface{}
	Limit      *int
	Offset     *int
	DistinctOn []string
	Fields     []string
}

// MutationParams represents parameters for a mutation
type MutationParams struct {
	TableName   string
	Object      map[string]interface{}
	Objects     []map[string]interface{}
	Where       map[string]interface{}
	SetValues   map[string]interface{}
	IncValues   map[string]interface{}
	OnConflict  map[string]interface{}
	PKColumns   map[string]interface{}
}

// ResolveQuery resolves a GraphQL query
func (r *Resolver) ResolveQuery(ctx context.Context, params QueryParams) ([]map[string]interface{}, error) {
	query, args := r.buildSelectQuery(params)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return r.scanRows(rows)
}

// ResolveQueryByPK resolves a query by primary key
func (r *Resolver) ResolveQueryByPK(ctx context.Context, tableName string, pkValues map[string]interface{}) (map[string]interface{}, error) {
	table := r.schema.Tables[tableName]
	if table == nil {
		return nil, fmt.Errorf("table %s not found", tableName)
	}

	var conditions []string
	var args []interface{}
	argIndex := 1

	for colName, value := range pkValues {
		conditions = append(conditions, fmt.Sprintf("%s.%s = $%d",
			quoteIdentifier(tableName), quoteIdentifier(colName), argIndex))
		args = append(args, value)
		argIndex++
	}

	query := fmt.Sprintf("SELECT * FROM %s.%s WHERE %s LIMIT 1",
		quoteIdentifier(r.dbSchema),
		quoteIdentifier(tableName),
		strings.Join(conditions, " AND "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	results, err := r.scanRows(rows)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// ResolveAggregate resolves an aggregate query
func (r *Resolver) ResolveAggregate(ctx context.Context, params QueryParams) (map[string]interface{}, error) {
	// Count query
	countQuery, countArgs := r.buildCountQuery(params)

	var count int
	err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("count query failed: %w", err)
	}

	// Get nodes if needed
	nodes, err := r.ResolveQuery(ctx, params)
	if err != nil {
		return nil, err
	}

	// Build aggregate result
	result := map[string]interface{}{
		"aggregate": map[string]interface{}{
			"count": count,
		},
		"nodes": nodes,
	}

	// Add min/max/sum/avg if there are numeric columns
	table := r.schema.Tables[params.TableName]
	if table != nil {
		aggFields, err := r.resolveAggregateFields(ctx, params, table)
		if err == nil {
			aggregate := result["aggregate"].(map[string]interface{})
			for k, v := range aggFields {
				aggregate[k] = v
			}
		}
	}

	return result, nil
}

// resolveAggregateFields resolves aggregate fields (min, max, sum, avg)
func (r *Resolver) resolveAggregateFields(ctx context.Context, params QueryParams, table *database.Table) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Find numeric columns
	var numericCols []string
	var comparableCols []string
	for _, col := range table.Columns {
		if isNumericType(col.SQLType) {
			numericCols = append(numericCols, col.Name)
			comparableCols = append(comparableCols, col.Name)
		} else if isComparableType(col.SQLType) {
			comparableCols = append(comparableCols, col.Name)
		}
	}

	if len(comparableCols) == 0 {
		return result, nil
	}

	// Build aggregate query
	var selectParts []string

	// Min/Max for comparable columns
	for _, col := range comparableCols {
		selectParts = append(selectParts,
			fmt.Sprintf("MIN(%s) as min_%s", quoteIdentifier(col), col),
			fmt.Sprintf("MAX(%s) as max_%s", quoteIdentifier(col), col))
	}

	// Sum/Avg/Stddev/Variance for numeric columns
	for _, col := range numericCols {
		selectParts = append(selectParts,
			fmt.Sprintf("SUM(%s) as sum_%s", quoteIdentifier(col), col),
			fmt.Sprintf("AVG(%s) as avg_%s", quoteIdentifier(col), col),
			fmt.Sprintf("STDDEV(%s) as stddev_%s", quoteIdentifier(col), col),
			fmt.Sprintf("VARIANCE(%s) as variance_%s", quoteIdentifier(col), col))
	}

	whereClause, args := r.buildWhereClause(params.Where, params.TableName, 1)

	query := fmt.Sprintf("SELECT %s FROM %s.%s",
		strings.Join(selectParts, ", "),
		quoteIdentifier(r.dbSchema),
		quoteIdentifier(params.TableName))

	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	if rows.Next() {
		columns, _ := rows.Columns()
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return result, err
		}

		// Organize results
		minFields := make(map[string]interface{})
		maxFields := make(map[string]interface{})
		sumFields := make(map[string]interface{})
		avgFields := make(map[string]interface{})
		stddevFields := make(map[string]interface{})
		varianceFields := make(map[string]interface{})

		for i, col := range columns {
			val := values[i]
			if strings.HasPrefix(col, "min_") {
				minFields[strings.TrimPrefix(col, "min_")] = val
			} else if strings.HasPrefix(col, "max_") {
				maxFields[strings.TrimPrefix(col, "max_")] = val
			} else if strings.HasPrefix(col, "sum_") {
				sumFields[strings.TrimPrefix(col, "sum_")] = val
			} else if strings.HasPrefix(col, "avg_") {
				avgFields[strings.TrimPrefix(col, "avg_")] = val
			} else if strings.HasPrefix(col, "stddev_") {
				stddevFields[strings.TrimPrefix(col, "stddev_")] = val
			} else if strings.HasPrefix(col, "variance_") {
				varianceFields[strings.TrimPrefix(col, "variance_")] = val
			}
		}

		if len(minFields) > 0 {
			result["min"] = minFields
		}
		if len(maxFields) > 0 {
			result["max"] = maxFields
		}
		if len(sumFields) > 0 {
			result["sum"] = sumFields
		}
		if len(avgFields) > 0 {
			result["avg"] = avgFields
		}
		if len(stddevFields) > 0 {
			result["stddev"] = stddevFields
		}
		if len(varianceFields) > 0 {
			result["variance"] = varianceFields
		}
	}

	return result, nil
}

// ResolveInsertOne resolves an insert_one mutation
func (r *Resolver) ResolveInsertOne(ctx context.Context, params MutationParams) (map[string]interface{}, error) {
	query, args := r.buildInsertQuery(params.TableName, params.Object)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("insert failed: %w", err)
	}
	defer rows.Close()

	results, err := r.scanRows(rows)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// ResolveInsert resolves an insert mutation
func (r *Resolver) ResolveInsert(ctx context.Context, params MutationParams) (map[string]interface{}, error) {
	if len(params.Objects) == 0 {
		return map[string]interface{}{
			"affected_rows": 0,
			"returning":     []map[string]interface{}{},
		}, nil
	}

	// Build batch insert query
	query, args := r.buildBatchInsertQuery(params.TableName, params.Objects)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("insert failed: %w", err)
	}
	defer rows.Close()

	results, err := r.scanRows(rows)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"affected_rows": len(results),
		"returning":     results,
	}, nil
}

// ResolveUpdateByPK resolves an update_by_pk mutation
func (r *Resolver) ResolveUpdateByPK(ctx context.Context, params MutationParams) (map[string]interface{}, error) {
	query, args := r.buildUpdateByPKQuery(params)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}
	defer rows.Close()

	results, err := r.scanRows(rows)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// ResolveUpdate resolves an update mutation
func (r *Resolver) ResolveUpdate(ctx context.Context, params MutationParams) (map[string]interface{}, error) {
	query, args := r.buildUpdateQuery(params)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}
	defer rows.Close()

	results, err := r.scanRows(rows)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"affected_rows": len(results),
		"returning":     results,
	}, nil
}

// ResolveDeleteByPK resolves a delete_by_pk mutation
func (r *Resolver) ResolveDeleteByPK(ctx context.Context, tableName string, pkValues map[string]interface{}) (map[string]interface{}, error) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	for colName, value := range pkValues {
		conditions = append(conditions, fmt.Sprintf("%s = $%d", quoteIdentifier(colName), argIndex))
		args = append(args, value)
		argIndex++
	}

	query := fmt.Sprintf("DELETE FROM %s.%s WHERE %s RETURNING *",
		quoteIdentifier(r.dbSchema),
		quoteIdentifier(tableName),
		strings.Join(conditions, " AND "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("delete failed: %w", err)
	}
	defer rows.Close()

	results, err := r.scanRows(rows)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// ResolveDelete resolves a delete mutation
func (r *Resolver) ResolveDelete(ctx context.Context, params MutationParams) (map[string]interface{}, error) {
	whereClause, args := r.buildWhereClause(params.Where, params.TableName, 1)

	query := fmt.Sprintf("DELETE FROM %s.%s",
		quoteIdentifier(r.dbSchema),
		quoteIdentifier(params.TableName))

	if whereClause != "" {
		query += " WHERE " + whereClause
	}
	query += " RETURNING *"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("delete failed: %w", err)
	}
	defer rows.Close()

	results, err := r.scanRows(rows)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"affected_rows": len(results),
		"returning":     results,
	}, nil
}

// buildSelectQuery builds a SELECT query
func (r *Resolver) buildSelectQuery(params QueryParams) (string, []interface{}) {
	var args []interface{}
	argIndex := 1

	// SELECT clause
	selectClause := "*"
	if len(params.DistinctOn) > 0 {
		distinctCols := make([]string, len(params.DistinctOn))
		for i, col := range params.DistinctOn {
			distinctCols[i] = quoteIdentifier(col)
		}
		selectClause = fmt.Sprintf("DISTINCT ON (%s) *", strings.Join(distinctCols, ", "))
	}

	query := fmt.Sprintf("SELECT %s FROM %s.%s",
		selectClause,
		quoteIdentifier(r.dbSchema),
		quoteIdentifier(params.TableName))

	// WHERE clause
	whereClause, whereArgs := r.buildWhereClause(params.Where, params.TableName, argIndex)
	if whereClause != "" {
		query += " WHERE " + whereClause
		args = append(args, whereArgs...)
		argIndex += len(whereArgs)
	}

	// ORDER BY clause
	if len(params.OrderBy) > 0 {
		orderParts := r.buildOrderByClause(params.OrderBy)
		if len(orderParts) > 0 {
			query += " ORDER BY " + strings.Join(orderParts, ", ")
		}
	}

	// LIMIT clause
	if params.Limit != nil {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, *params.Limit)
		argIndex++
	}

	// OFFSET clause
	if params.Offset != nil {
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, *params.Offset)
		argIndex++
	}

	return query, args
}

// buildCountQuery builds a COUNT query
func (r *Resolver) buildCountQuery(params QueryParams) (string, []interface{}) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s.%s",
		quoteIdentifier(r.dbSchema),
		quoteIdentifier(params.TableName))

	whereClause, args := r.buildWhereClause(params.Where, params.TableName, 1)
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	return query, args
}

// buildWhereClause builds a WHERE clause from filter conditions
func (r *Resolver) buildWhereClause(where map[string]interface{}, tableName string, startIndex int) (string, []interface{}) {
	if len(where) == 0 {
		return "", nil
	}

	var conditions []string
	var args []interface{}
	argIndex := startIndex

	for key, value := range where {
		switch key {
		case "_and":
			if andConditions, ok := value.([]interface{}); ok {
				var andParts []string
				for _, cond := range andConditions {
					if condMap, ok := cond.(map[string]interface{}); ok {
						subClause, subArgs := r.buildWhereClause(condMap, tableName, argIndex)
						if subClause != "" {
							andParts = append(andParts, "("+subClause+")")
							args = append(args, subArgs...)
							argIndex += len(subArgs)
						}
					}
				}
				if len(andParts) > 0 {
					conditions = append(conditions, "("+strings.Join(andParts, " AND ")+")")
				}
			}

		case "_or":
			if orConditions, ok := value.([]interface{}); ok {
				var orParts []string
				for _, cond := range orConditions {
					if condMap, ok := cond.(map[string]interface{}); ok {
						subClause, subArgs := r.buildWhereClause(condMap, tableName, argIndex)
						if subClause != "" {
							orParts = append(orParts, "("+subClause+")")
							args = append(args, subArgs...)
							argIndex += len(subArgs)
						}
					}
				}
				if len(orParts) > 0 {
					conditions = append(conditions, "("+strings.Join(orParts, " OR ")+")")
				}
			}

		case "_not":
			if notCond, ok := value.(map[string]interface{}); ok {
				subClause, subArgs := r.buildWhereClause(notCond, tableName, argIndex)
				if subClause != "" {
					conditions = append(conditions, "NOT ("+subClause+")")
					args = append(args, subArgs...)
					argIndex += len(subArgs)
				}
			}

		default:
			// Column comparison
			if compOps, ok := value.(map[string]interface{}); ok {
				colConditions, colArgs := r.buildColumnConditions(key, compOps, argIndex)
				conditions = append(conditions, colConditions...)
				args = append(args, colArgs...)
				argIndex += len(colArgs)
			}
		}
	}

	return strings.Join(conditions, " AND "), args
}

// buildColumnConditions builds conditions for a single column
func (r *Resolver) buildColumnConditions(column string, ops map[string]interface{}, startIndex int) ([]string, []interface{}) {
	var conditions []string
	var args []interface{}
	argIndex := startIndex
	quotedCol := quoteIdentifier(column)

	for op, value := range ops {
		switch op {
		case "_eq":
			conditions = append(conditions, fmt.Sprintf("%s = $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_neq":
			conditions = append(conditions, fmt.Sprintf("%s != $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_gt":
			conditions = append(conditions, fmt.Sprintf("%s > $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_gte":
			conditions = append(conditions, fmt.Sprintf("%s >= $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_lt":
			conditions = append(conditions, fmt.Sprintf("%s < $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_lte":
			conditions = append(conditions, fmt.Sprintf("%s <= $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_in":
			if arr, ok := value.([]interface{}); ok && len(arr) > 0 {
				placeholders := make([]string, len(arr))
				for i, v := range arr {
					placeholders[i] = fmt.Sprintf("$%d", argIndex)
					args = append(args, v)
					argIndex++
				}
				conditions = append(conditions, fmt.Sprintf("%s IN (%s)", quotedCol, strings.Join(placeholders, ", ")))
			}
		case "_nin":
			if arr, ok := value.([]interface{}); ok && len(arr) > 0 {
				placeholders := make([]string, len(arr))
				for i, v := range arr {
					placeholders[i] = fmt.Sprintf("$%d", argIndex)
					args = append(args, v)
					argIndex++
				}
				conditions = append(conditions, fmt.Sprintf("%s NOT IN (%s)", quotedCol, strings.Join(placeholders, ", ")))
			}
		case "_is_null":
			if isNull, ok := value.(bool); ok {
				if isNull {
					conditions = append(conditions, fmt.Sprintf("%s IS NULL", quotedCol))
				} else {
					conditions = append(conditions, fmt.Sprintf("%s IS NOT NULL", quotedCol))
				}
			}
		case "_like":
			conditions = append(conditions, fmt.Sprintf("%s LIKE $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_nlike":
			conditions = append(conditions, fmt.Sprintf("%s NOT LIKE $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_ilike":
			conditions = append(conditions, fmt.Sprintf("%s ILIKE $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_nilike":
			conditions = append(conditions, fmt.Sprintf("%s NOT ILIKE $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_similar":
			conditions = append(conditions, fmt.Sprintf("%s SIMILAR TO $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_nsimilar":
			conditions = append(conditions, fmt.Sprintf("%s NOT SIMILAR TO $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_regex":
			conditions = append(conditions, fmt.Sprintf("%s ~ $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_nregex":
			conditions = append(conditions, fmt.Sprintf("%s !~ $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_iregex":
			conditions = append(conditions, fmt.Sprintf("%s ~* $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_niregex":
			conditions = append(conditions, fmt.Sprintf("%s !~* $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_contains":
			conditions = append(conditions, fmt.Sprintf("%s @> $%d", quotedCol, argIndex))
			jsonBytes, _ := json.Marshal(value)
			args = append(args, string(jsonBytes))
			argIndex++
		case "_contained_in":
			conditions = append(conditions, fmt.Sprintf("%s <@ $%d", quotedCol, argIndex))
			jsonBytes, _ := json.Marshal(value)
			args = append(args, string(jsonBytes))
			argIndex++
		case "_has_key":
			conditions = append(conditions, fmt.Sprintf("%s ? $%d", quotedCol, argIndex))
			args = append(args, value)
			argIndex++
		case "_has_keys_any":
			if arr, ok := value.([]interface{}); ok {
				strArr := make([]string, len(arr))
				for i, v := range arr {
					strArr[i] = fmt.Sprintf("%v", v)
				}
				conditions = append(conditions, fmt.Sprintf("%s ?| $%d", quotedCol, argIndex))
				args = append(args, "{"+strings.Join(strArr, ",")+"}")
				argIndex++
			}
		case "_has_keys_all":
			if arr, ok := value.([]interface{}); ok {
				strArr := make([]string, len(arr))
				for i, v := range arr {
					strArr[i] = fmt.Sprintf("%v", v)
				}
				conditions = append(conditions, fmt.Sprintf("%s ?& $%d", quotedCol, argIndex))
				args = append(args, "{"+strings.Join(strArr, ",")+"}")
				argIndex++
			}
		}
	}

	return conditions, args
}

// buildOrderByClause builds an ORDER BY clause
func (r *Resolver) buildOrderByClause(orderBy []map[string]interface{}) []string {
	var parts []string

	for _, ob := range orderBy {
		for col, direction := range ob {
			dir := "ASC"
			switch v := direction.(type) {
			case string:
				dir = v
			}
			parts = append(parts, fmt.Sprintf("%s %s", quoteIdentifier(col), dir))
		}
	}

	return parts
}

// buildInsertQuery builds an INSERT query
func (r *Resolver) buildInsertQuery(tableName string, object map[string]interface{}) (string, []interface{}) {
	var columns []string
	var placeholders []string
	var args []interface{}
	argIndex := 1

	for col, val := range object {
		columns = append(columns, quoteIdentifier(col))
		placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
		args = append(args, convertValue(val))
		argIndex++
	}

	query := fmt.Sprintf("INSERT INTO %s.%s (%s) VALUES (%s) RETURNING *",
		quoteIdentifier(r.dbSchema),
		quoteIdentifier(tableName),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	return query, args
}

// buildBatchInsertQuery builds a batch INSERT query
func (r *Resolver) buildBatchInsertQuery(tableName string, objects []map[string]interface{}) (string, []interface{}) {
	if len(objects) == 0 {
		return "", nil
	}

	// Get columns from first object
	var columns []string
	for col := range objects[0] {
		columns = append(columns, col)
	}

	var valueSets []string
	var args []interface{}
	argIndex := 1

	for _, obj := range objects {
		var placeholders []string
		for _, col := range columns {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
			args = append(args, convertValue(obj[col]))
			argIndex++
		}
		valueSets = append(valueSets, "("+strings.Join(placeholders, ", ")+")")
	}

	quotedColumns := make([]string, len(columns))
	for i, col := range columns {
		quotedColumns[i] = quoteIdentifier(col)
	}

	query := fmt.Sprintf("INSERT INTO %s.%s (%s) VALUES %s RETURNING *",
		quoteIdentifier(r.dbSchema),
		quoteIdentifier(tableName),
		strings.Join(quotedColumns, ", "),
		strings.Join(valueSets, ", "))

	return query, args
}

// buildUpdateByPKQuery builds an UPDATE by primary key query
func (r *Resolver) buildUpdateByPKQuery(params MutationParams) (string, []interface{}) {
	var setParts []string
	var args []interface{}
	argIndex := 1

	// Handle _set
	for col, val := range params.SetValues {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", quoteIdentifier(col), argIndex))
		args = append(args, convertValue(val))
		argIndex++
	}

	// Handle _inc
	for col, val := range params.IncValues {
		setParts = append(setParts, fmt.Sprintf("%s = %s + $%d", quoteIdentifier(col), quoteIdentifier(col), argIndex))
		args = append(args, val)
		argIndex++
	}

	// Build WHERE clause for PK
	var conditions []string
	for col, val := range params.PKColumns {
		conditions = append(conditions, fmt.Sprintf("%s = $%d", quoteIdentifier(col), argIndex))
		args = append(args, val)
		argIndex++
	}

	query := fmt.Sprintf("UPDATE %s.%s SET %s WHERE %s RETURNING *",
		quoteIdentifier(r.dbSchema),
		quoteIdentifier(params.TableName),
		strings.Join(setParts, ", "),
		strings.Join(conditions, " AND "))

	return query, args
}

// buildUpdateQuery builds an UPDATE query
func (r *Resolver) buildUpdateQuery(params MutationParams) (string, []interface{}) {
	var setParts []string
	var args []interface{}
	argIndex := 1

	// Handle _set
	for col, val := range params.SetValues {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", quoteIdentifier(col), argIndex))
		args = append(args, convertValue(val))
		argIndex++
	}

	// Handle _inc
	for col, val := range params.IncValues {
		setParts = append(setParts, fmt.Sprintf("%s = %s + $%d", quoteIdentifier(col), quoteIdentifier(col), argIndex))
		args = append(args, val)
		argIndex++
	}

	query := fmt.Sprintf("UPDATE %s.%s SET %s",
		quoteIdentifier(r.dbSchema),
		quoteIdentifier(params.TableName),
		strings.Join(setParts, ", "))

	// Build WHERE clause
	whereClause, whereArgs := r.buildWhereClause(params.Where, params.TableName, argIndex)
	if whereClause != "" {
		query += " WHERE " + whereClause
		args = append(args, whereArgs...)
	}

	query += " RETURNING *"

	return query, args
}

// scanRows scans rows into a slice of maps
func (r *Resolver) scanRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Handle special types
			switch v := val.(type) {
			case []byte:
				// Try to parse as JSON
				var jsonVal interface{}
				if err := json.Unmarshal(v, &jsonVal); err == nil {
					row[col] = jsonVal
				} else {
					row[col] = string(v)
				}
			default:
				row[col] = v
			}
		}
		results = append(results, row)
	}

	return results, nil
}

// quoteIdentifier quotes a PostgreSQL identifier
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// convertValue converts a value for PostgreSQL
func convertValue(val interface{}) interface{} {
	switch v := val.(type) {
	case map[string]interface{}, []interface{}:
		jsonBytes, _ := json.Marshal(v)
		return string(jsonBytes)
	default:
		return v
	}
}

// isNumericType checks if a SQL type is numeric
func isNumericType(sqlType string) bool {
	switch strings.ToLower(sqlType) {
	case "int2", "int4", "int8", "smallint", "integer", "bigint",
		"smallserial", "serial", "bigserial",
		"float4", "float8", "real", "double precision",
		"numeric", "decimal", "money":
		return true
	default:
		return false
	}
}

// isComparableType checks if a SQL type is comparable
func isComparableType(sqlType string) bool {
	if isNumericType(sqlType) {
		return true
	}

	switch strings.ToLower(sqlType) {
	case "date", "time", "timetz", "timestamp", "timestamptz",
		"varchar", "char", "text", "uuid":
		return true
	default:
		return false
	}
}
