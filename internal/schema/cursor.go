package schema

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// PageInfo contains information about pagination
type PageInfo struct {
	HasNextPage     bool    `json:"hasNextPage"`
	HasPreviousPage bool    `json:"hasPreviousPage"`
	StartCursor     *string `json:"startCursor"`
	EndCursor       *string `json:"endCursor"`
}

// Edge represents a single item in a connection with its cursor
type Edge struct {
	Node   map[string]interface{} `json:"node"`
	Cursor string                 `json:"cursor"`
}

// Connection represents a paginated list of edges
type Connection struct {
	Edges      []Edge   `json:"edges"`
	PageInfo   PageInfo `json:"pageInfo"`
	TotalCount *int     `json:"totalCount,omitempty"`
}

// CursorData holds the data encoded in a cursor
type CursorData struct {
	PrimaryKey map[string]interface{} `json:"pk"`
	OrderBy    []map[string]interface{} `json:"order,omitempty"`
}

// EncodeCursor encodes cursor data to a base64 string
func EncodeCursor(data CursorData) (string, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cursor data: %w", err)
	}
	return base64.StdEncoding.EncodeToString(jsonBytes), nil
}

// DecodeCursor decodes a base64 cursor string to cursor data
func DecodeCursor(cursor string) (*CursorData, error) {
	if cursor == "" {
		return nil, nil
	}

	jsonBytes, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor format: %w", err)
	}

	var data CursorData
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor data: %w", err)
	}

	return &data, nil
}

// BuildCursorFromRow creates a cursor from a database row and primary key columns
func BuildCursorFromRow(row map[string]interface{}, pkColumns []string, orderBy []map[string]interface{}) (string, error) {
	pk := make(map[string]interface{})
	for _, col := range pkColumns {
		if val, ok := row[col]; ok {
			pk[col] = val
		}
	}

	if len(pk) == 0 {
		return "", fmt.Errorf("no primary key values found in row")
	}

	data := CursorData{
		PrimaryKey: pk,
		OrderBy:    orderBy,
	}

	return EncodeCursor(data)
}

// ApplyCursorToWhere applies cursor constraints to the WHERE clause
func ApplyCursorToWhere(where map[string]interface{}, cursor *CursorData, isAfter bool, pkColumns []string) map[string]interface{} {
	if cursor == nil || len(cursor.PrimaryKey) == 0 {
		return where
	}

	if where == nil {
		where = make(map[string]interface{})
	}

	// For simple single-column primary keys, use simple comparison
	if len(pkColumns) == 1 {
		pkCol := pkColumns[0]
		if pkValue, ok := cursor.PrimaryKey[pkCol]; ok {
			operator := "_gt"
			if !isAfter {
				operator = "_lt"
			}

			// If there's already a filter on this column, combine with AND
			if existingFilter, hasFilter := where[pkCol]; hasFilter {
				where["_and"] = []interface{}{
					map[string]interface{}{pkCol: existingFilter},
					map[string]interface{}{pkCol: map[string]interface{}{operator: pkValue}},
				}
				delete(where, pkCol)
			} else {
				where[pkCol] = map[string]interface{}{operator: pkValue}
			}
		}
	} else {
		// For composite keys, we need to build a more complex condition
		// This is a simplified implementation; production may need tuple comparison
		andConditions := []interface{}{}

		for _, pkCol := range pkColumns {
			if pkValue, ok := cursor.PrimaryKey[pkCol]; ok {
				operator := "_gt"
				if !isAfter {
					operator = "_lt"
				}
				andConditions = append(andConditions, map[string]interface{}{
					pkCol: map[string]interface{}{operator: pkValue},
				})
			}
		}

		if len(andConditions) > 0 {
			if existingAnd, hasAnd := where["_and"]; hasAnd {
				// Combine with existing AND conditions
				if andSlice, ok := existingAnd.([]interface{}); ok {
					andConditions = append(andConditions, andSlice...)
				}
			}
			where["_and"] = andConditions
		}
	}

	return where
}

// BuildConnection creates a Connection from query results
func BuildConnection(
	rows []map[string]interface{},
	pkColumns []string,
	orderBy []map[string]interface{},
	first *int,
	last *int,
	totalCount *int,
) (*Connection, error) {
	if len(rows) == 0 {
		return &Connection{
			Edges:      []Edge{},
			PageInfo:   PageInfo{HasNextPage: false, HasPreviousPage: false},
			TotalCount: totalCount,
		}, nil
	}

	// Build edges with cursors
	edges := make([]Edge, len(rows))
	for i, row := range rows {
		cursor, err := BuildCursorFromRow(row, pkColumns, orderBy)
		if err != nil {
			return nil, fmt.Errorf("failed to build cursor for row %d: %w", i, err)
		}
		edges[i] = Edge{
			Node:   row,
			Cursor: cursor,
		}
	}

	// Determine if there are more pages
	hasNextPage := false
	hasPreviousPage := false

	if first != nil && len(edges) > *first {
		hasNextPage = true
		edges = edges[:*first]
	}

	if last != nil && len(edges) > *last {
		hasPreviousPage = true
		edges = edges[len(edges)-*last:]
	}

	// Get start and end cursors
	var startCursor, endCursor *string
	if len(edges) > 0 {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &Connection{
		Edges: edges,
		PageInfo: PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: hasPreviousPage,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: totalCount,
	}, nil
}
