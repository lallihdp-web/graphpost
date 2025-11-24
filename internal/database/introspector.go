package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Introspector handles database schema introspection
type Introspector struct {
	pool   *pgxpool.Pool
	schema string
}

// Schema represents the complete database schema
type Schema struct {
	Tables      map[string]*Table
	Views       map[string]*View
	Functions   map[string]*Function
	Enums       map[string]*Enum
	Sequences   map[string]*Sequence
	ForeignKeys []*ForeignKey
	Indexes     map[string][]*Index
	Triggers    map[string][]*Trigger
}

// Table represents a database table
type Table struct {
	Name        string
	Schema      string
	Columns     []*Column
	PrimaryKey  *PrimaryKey
	ForeignKeys []*ForeignKey
	Indexes     []*Index
	Constraints []*Constraint
	Comment     string
	IsPartition bool
}

// View represents a database view
type View struct {
	Name           string
	Schema         string
	Columns        []*Column
	Definition     string
	Comment        string
	IsMaterialized bool
}

// Column represents a table column
type Column struct {
	Name         string
	Type         string
	SQLType      string
	IsNullable   bool
	HasDefault   bool
	DefaultValue *string
	IsPrimaryKey bool
	IsUnique     bool
	IsArray      bool
	ArrayDims    int
	Comment      string
	Position     int
	MaxLength    *int
	Precision    *int
	Scale        *int
	EnumValues   []string
}

// PrimaryKey represents a primary key constraint
type PrimaryKey struct {
	Name    string
	Columns []string
}

// ForeignKey represents a foreign key relationship
type ForeignKey struct {
	Name          string
	SourceTable   string
	SourceSchema  string
	SourceColumns []string
	TargetTable   string
	TargetSchema  string
	TargetColumns []string
	OnDelete      string
	OnUpdate      string
}

// Index represents a database index
type Index struct {
	Name       string
	Table      string
	Columns    []string
	IsUnique   bool
	IsPrimary  bool
	Type       string
	Definition string
}

// Constraint represents a database constraint
type Constraint struct {
	Name       string
	Type       string // PRIMARY KEY, UNIQUE, CHECK, FOREIGN KEY
	Columns    []string
	Definition string
}

// Function represents a database function
type Function struct {
	Name       string
	Schema     string
	ReturnType string
	Arguments  []*FunctionArg
	Language   string
	Definition string
	IsVolatile bool
}

// FunctionArg represents a function argument
type FunctionArg struct {
	Name    string
	Type    string
	Mode    string // IN, OUT, INOUT
	Default *string
}

// Enum represents a custom enum type
type Enum struct {
	Name   string
	Schema string
	Values []string
}

// Sequence represents a database sequence
type Sequence struct {
	Name      string
	Schema    string
	DataType  string
	Start     int64
	Increment int64
	MinValue  int64
	MaxValue  int64
	Cache     int64
	Cycle     bool
}

// Trigger represents a database trigger
type Trigger struct {
	Name       string
	Table      string
	Schema     string
	Event      string
	Timing     string
	Definition string
	Enabled    bool
}

// NewIntrospector creates a new database introspector
func NewIntrospector(pool *pgxpool.Pool, schema string) *Introspector {
	if schema == "" {
		schema = "public"
	}
	return &Introspector{
		pool:   pool,
		schema: schema,
	}
}

// IntrospectSchema performs complete database introspection
func (i *Introspector) IntrospectSchema(ctx context.Context) (*Schema, error) {
	schema := &Schema{
		Tables:    make(map[string]*Table),
		Views:     make(map[string]*View),
		Functions: make(map[string]*Function),
		Enums:     make(map[string]*Enum),
		Sequences: make(map[string]*Sequence),
		Indexes:   make(map[string][]*Index),
		Triggers:  make(map[string][]*Trigger),
	}

	// Introspect enums first (needed for column types)
	enums, err := i.IntrospectEnums(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect enums: %w", err)
	}
	schema.Enums = enums

	// Introspect tables
	tables, err := i.IntrospectTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect tables: %w", err)
	}
	schema.Tables = tables

	// Introspect views
	views, err := i.IntrospectViews(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect views: %w", err)
	}
	schema.Views = views

	// Introspect foreign keys
	foreignKeys, err := i.IntrospectForeignKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect foreign keys: %w", err)
	}
	schema.ForeignKeys = foreignKeys

	// Associate foreign keys with tables
	for _, fk := range foreignKeys {
		if table, ok := schema.Tables[fk.SourceTable]; ok {
			table.ForeignKeys = append(table.ForeignKeys, fk)
		}
	}

	// Introspect functions
	functions, err := i.IntrospectFunctions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect functions: %w", err)
	}
	schema.Functions = functions

	// Introspect sequences
	sequences, err := i.IntrospectSequences(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect sequences: %w", err)
	}
	schema.Sequences = sequences

	return schema, nil
}

// IntrospectTables retrieves all tables in the schema
func (i *Introspector) IntrospectTables(ctx context.Context) (map[string]*Table, error) {
	query := `
		SELECT
			t.table_name,
			t.table_schema,
			obj_description((t.table_schema || '.' || t.table_name)::regclass) as comment
		FROM information_schema.tables t
		WHERE t.table_schema = $1
			AND t.table_type = 'BASE TABLE'
		ORDER BY t.table_name
	`

	rows, err := i.pool.Query(ctx, query, i.schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := make(map[string]*Table)
	for rows.Next() {
		var tableName, tableSchema string
		var comment *string

		if err := rows.Scan(&tableName, &tableSchema, &comment); err != nil {
			return nil, err
		}

		table := &Table{
			Name:   tableName,
			Schema: tableSchema,
		}
		if comment != nil {
			table.Comment = *comment
		}

		// Get columns for this table
		columns, err := i.IntrospectColumns(ctx, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get columns for table %s: %w", tableName, err)
		}
		table.Columns = columns

		// Get primary key
		pk, err := i.IntrospectPrimaryKey(ctx, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get primary key for table %s: %w", tableName, err)
		}
		table.PrimaryKey = pk

		// Mark primary key columns
		if pk != nil {
			pkCols := make(map[string]bool)
			for _, col := range pk.Columns {
				pkCols[col] = true
			}
			for _, col := range table.Columns {
				if pkCols[col.Name] {
					col.IsPrimaryKey = true
				}
			}
		}

		// Get indexes
		indexes, err := i.IntrospectIndexes(ctx, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get indexes for table %s: %w", tableName, err)
		}
		table.Indexes = indexes

		tables[tableName] = table
	}

	return tables, rows.Err()
}

// IntrospectColumns retrieves all columns for a table
func (i *Introspector) IntrospectColumns(ctx context.Context, tableName string) ([]*Column, error) {
	query := `
		SELECT
			c.column_name,
			c.data_type,
			c.udt_name,
			c.is_nullable = 'YES' as is_nullable,
			c.column_default IS NOT NULL as has_default,
			c.column_default,
			c.ordinal_position,
			c.character_maximum_length,
			c.numeric_precision,
			c.numeric_scale,
			col_description((c.table_schema || '.' || c.table_name)::regclass, c.ordinal_position) as comment
		FROM information_schema.columns c
		WHERE c.table_schema = $1
			AND c.table_name = $2
		ORDER BY c.ordinal_position
	`

	rows, err := i.pool.Query(ctx, query, i.schema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []*Column
	for rows.Next() {
		var col Column
		var defaultValue, comment *string
		var maxLength, precision, scale *int64

		if err := rows.Scan(
			&col.Name,
			&col.Type,
			&col.SQLType,
			&col.IsNullable,
			&col.HasDefault,
			&defaultValue,
			&col.Position,
			&maxLength,
			&precision,
			&scale,
			&comment,
		); err != nil {
			return nil, err
		}

		if defaultValue != nil {
			col.DefaultValue = defaultValue
		}
		if maxLength != nil {
			ml := int(*maxLength)
			col.MaxLength = &ml
		}
		if precision != nil {
			p := int(*precision)
			col.Precision = &p
		}
		if scale != nil {
			s := int(*scale)
			col.Scale = &s
		}
		if comment != nil {
			col.Comment = *comment
		}

		// Check if array type
		if strings.HasPrefix(col.SQLType, "_") {
			col.IsArray = true
			col.SQLType = strings.TrimPrefix(col.SQLType, "_")
			col.ArrayDims = 1
		}

		columns = append(columns, &col)
	}

	return columns, rows.Err()
}

// IntrospectPrimaryKey retrieves the primary key for a table
func (i *Introspector) IntrospectPrimaryKey(ctx context.Context, tableName string) (*PrimaryKey, error) {
	query := `
		SELECT
			tc.constraint_name,
			kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.table_schema = $1
			AND tc.table_name = $2
			AND tc.constraint_type = 'PRIMARY KEY'
		ORDER BY kcu.ordinal_position
	`

	rows, err := i.pool.Query(ctx, query, i.schema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pk *PrimaryKey
	for rows.Next() {
		var constraintName, columnName string
		if err := rows.Scan(&constraintName, &columnName); err != nil {
			return nil, err
		}

		if pk == nil {
			pk = &PrimaryKey{
				Name:    constraintName,
				Columns: []string{},
			}
		}
		pk.Columns = append(pk.Columns, columnName)
	}

	return pk, rows.Err()
}

// IntrospectForeignKeys retrieves all foreign keys in the schema
func (i *Introspector) IntrospectForeignKeys(ctx context.Context) ([]*ForeignKey, error) {
	query := `
		SELECT
			tc.constraint_name,
			tc.table_name as source_table,
			tc.table_schema as source_schema,
			kcu.column_name as source_column,
			ccu.table_name as target_table,
			ccu.table_schema as target_schema,
			ccu.column_name as target_column,
			rc.delete_rule,
			rc.update_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_name = ccu.constraint_name
			AND tc.table_schema = ccu.table_schema
		JOIN information_schema.referential_constraints rc
			ON tc.constraint_name = rc.constraint_name
			AND tc.table_schema = rc.constraint_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = $1
		ORDER BY tc.constraint_name, kcu.ordinal_position
	`

	rows, err := i.pool.Query(ctx, query, i.schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fkMap := make(map[string]*ForeignKey)
	var fkOrder []string

	for rows.Next() {
		var (
			constraintName, sourceTable, sourceSchema, sourceColumn string
			targetTable, targetSchema, targetColumn                 string
			deleteRule, updateRule                                  string
		)

		if err := rows.Scan(
			&constraintName,
			&sourceTable, &sourceSchema, &sourceColumn,
			&targetTable, &targetSchema, &targetColumn,
			&deleteRule, &updateRule,
		); err != nil {
			return nil, err
		}

		if fk, exists := fkMap[constraintName]; exists {
			fk.SourceColumns = append(fk.SourceColumns, sourceColumn)
			fk.TargetColumns = append(fk.TargetColumns, targetColumn)
		} else {
			fkMap[constraintName] = &ForeignKey{
				Name:          constraintName,
				SourceTable:   sourceTable,
				SourceSchema:  sourceSchema,
				SourceColumns: []string{sourceColumn},
				TargetTable:   targetTable,
				TargetSchema:  targetSchema,
				TargetColumns: []string{targetColumn},
				OnDelete:      deleteRule,
				OnUpdate:      updateRule,
			}
			fkOrder = append(fkOrder, constraintName)
		}
	}

	var foreignKeys []*ForeignKey
	for _, name := range fkOrder {
		foreignKeys = append(foreignKeys, fkMap[name])
	}

	return foreignKeys, rows.Err()
}

// IntrospectViews retrieves all views in the schema
func (i *Introspector) IntrospectViews(ctx context.Context) (map[string]*View, error) {
	query := `
		SELECT
			v.table_name,
			v.table_schema,
			pg_get_viewdef((v.table_schema || '.' || v.table_name)::regclass) as definition,
			obj_description((v.table_schema || '.' || v.table_name)::regclass) as comment,
			EXISTS (
				SELECT 1 FROM pg_matviews m
				WHERE m.schemaname = v.table_schema AND m.matviewname = v.table_name
			) as is_materialized
		FROM information_schema.views v
		WHERE v.table_schema = $1
		ORDER BY v.table_name
	`

	rows, err := i.pool.Query(ctx, query, i.schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	views := make(map[string]*View)
	for rows.Next() {
		var viewName, viewSchema string
		var definition, comment *string
		var isMaterialized bool

		if err := rows.Scan(&viewName, &viewSchema, &definition, &comment, &isMaterialized); err != nil {
			return nil, err
		}

		view := &View{
			Name:           viewName,
			Schema:         viewSchema,
			IsMaterialized: isMaterialized,
		}
		if definition != nil {
			view.Definition = *definition
		}
		if comment != nil {
			view.Comment = *comment
		}

		// Get columns for this view
		columns, err := i.IntrospectColumns(ctx, viewName)
		if err != nil {
			return nil, fmt.Errorf("failed to get columns for view %s: %w", viewName, err)
		}
		view.Columns = columns

		views[viewName] = view
	}

	return views, rows.Err()
}

// IntrospectEnums retrieves all enum types in the schema
func (i *Introspector) IntrospectEnums(ctx context.Context) (map[string]*Enum, error) {
	query := `
		SELECT
			t.typname as enum_name,
			n.nspname as enum_schema,
			array_agg(e.enumlabel ORDER BY e.enumsortorder) as enum_values
		FROM pg_type t
		JOIN pg_enum e ON t.oid = e.enumtypid
		JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE n.nspname = $1
		GROUP BY t.typname, n.nspname
		ORDER BY t.typname
	`

	rows, err := i.pool.Query(ctx, query, i.schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	enums := make(map[string]*Enum)
	for rows.Next() {
		var enumName, enumSchema string
		var enumValues []string

		if err := rows.Scan(&enumName, &enumSchema, &enumValues); err != nil {
			return nil, err
		}

		enums[enumName] = &Enum{
			Name:   enumName,
			Schema: enumSchema,
			Values: enumValues,
		}
	}

	return enums, rows.Err()
}

// IntrospectIndexes retrieves all indexes for a table
func (i *Introspector) IntrospectIndexes(ctx context.Context, tableName string) ([]*Index, error) {
	query := `
		SELECT
			i.relname as index_name,
			t.relname as table_name,
			array_agg(a.attname ORDER BY k.n) as column_names,
			ix.indisunique as is_unique,
			ix.indisprimary as is_primary,
			am.amname as index_type,
			pg_get_indexdef(ix.indexrelid) as definition
		FROM pg_index ix
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN pg_class t ON t.oid = ix.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_am am ON am.oid = i.relam
		JOIN LATERAL unnest(ix.indkey) WITH ORDINALITY AS k(attnum, n) ON true
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum
		WHERE n.nspname = $1
			AND t.relname = $2
		GROUP BY i.relname, t.relname, ix.indisunique, ix.indisprimary, am.amname, ix.indexrelid
		ORDER BY i.relname
	`

	rows, err := i.pool.Query(ctx, query, i.schema, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []*Index
	for rows.Next() {
		var idx Index
		var columns []string

		if err := rows.Scan(
			&idx.Name,
			&idx.Table,
			&columns,
			&idx.IsUnique,
			&idx.IsPrimary,
			&idx.Type,
			&idx.Definition,
		); err != nil {
			return nil, err
		}
		idx.Columns = columns

		indexes = append(indexes, &idx)
	}

	return indexes, rows.Err()
}

// IntrospectFunctions retrieves all functions in the schema
func (i *Introspector) IntrospectFunctions(ctx context.Context) (map[string]*Function, error) {
	query := `
		SELECT
			p.proname as function_name,
			n.nspname as function_schema,
			pg_get_function_result(p.oid) as return_type,
			l.lanname as language,
			p.prosrc as definition,
			p.provolatile = 'v' as is_volatile
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		JOIN pg_language l ON l.oid = p.prolang
		WHERE n.nspname = $1
			AND p.prokind = 'f'
		ORDER BY p.proname
	`

	rows, err := i.pool.Query(ctx, query, i.schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	functions := make(map[string]*Function)
	for rows.Next() {
		var fn Function
		var definition *string

		if err := rows.Scan(
			&fn.Name,
			&fn.Schema,
			&fn.ReturnType,
			&fn.Language,
			&definition,
			&fn.IsVolatile,
		); err != nil {
			return nil, err
		}
		if definition != nil {
			fn.Definition = *definition
		}

		functions[fn.Name] = &fn
	}

	return functions, rows.Err()
}

// IntrospectSequences retrieves all sequences in the schema
func (i *Introspector) IntrospectSequences(ctx context.Context) (map[string]*Sequence, error) {
	query := `
		SELECT
			s.sequence_name,
			s.sequence_schema,
			s.data_type,
			s.start_value::bigint,
			s.increment::bigint,
			s.minimum_value::bigint,
			s.maximum_value::bigint,
			COALESCE(s.cycle_option = 'YES', false) as cycle
		FROM information_schema.sequences s
		WHERE s.sequence_schema = $1
		ORDER BY s.sequence_name
	`

	rows, err := i.pool.Query(ctx, query, i.schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sequences := make(map[string]*Sequence)
	for rows.Next() {
		var seq Sequence

		if err := rows.Scan(
			&seq.Name,
			&seq.Schema,
			&seq.DataType,
			&seq.Start,
			&seq.Increment,
			&seq.MinValue,
			&seq.MaxValue,
			&seq.Cycle,
		); err != nil {
			return nil, err
		}

		sequences[seq.Name] = &seq
	}

	return sequences, rows.Err()
}

// CollectRows is a helper function to collect rows into a slice
func CollectRows[T any](rows pgx.Rows, fn func(row pgx.CollectableRow) (T, error)) ([]T, error) {
	return pgx.CollectRows(rows, fn)
}
