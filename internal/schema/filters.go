package schema

import (
	"strings"

	"github.com/graphpost/graphpost/internal/database"
	"github.com/graphql-go/graphql"
)

// FilterGenerator generates filter input types for tables
type FilterGenerator struct {
	dbSchema    *database.Schema
	enumTypes   map[string]*graphql.Enum
	filterTypes map[string]*graphql.InputObject
}

// NewFilterGenerator creates a new filter generator
func NewFilterGenerator(dbSchema *database.Schema, enumTypes map[string]*graphql.Enum) *FilterGenerator {
	return &FilterGenerator{
		dbSchema:    dbSchema,
		enumTypes:   enumTypes,
		filterTypes: make(map[string]*graphql.InputObject),
	}
}

// Generate creates all filter input types
func (fg *FilterGenerator) Generate() map[string]*graphql.InputObject {
	// Generate comparison input types for each scalar type
	fg.generateComparisonTypes()

	// Generate bool_exp for each table
	for tableName := range fg.dbSchema.Tables {
		fg.generateBoolExp(tableName)
	}

	// Generate bool_exp for each view
	for viewName := range fg.dbSchema.Views {
		fg.generateBoolExpForView(viewName)
	}

	return fg.filterTypes
}

// generateComparisonTypes creates comparison input types for each scalar type
func (fg *FilterGenerator) generateComparisonTypes() {
	// String comparison
	fg.filterTypes["String_comparison_exp"] = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "String_comparison_exp",
		Fields: graphql.InputObjectConfigFieldMap{
			"_eq":       {Type: graphql.String, Description: "Equal to"},
			"_neq":      {Type: graphql.String, Description: "Not equal to"},
			"_gt":       {Type: graphql.String, Description: "Greater than"},
			"_gte":      {Type: graphql.String, Description: "Greater than or equal to"},
			"_lt":       {Type: graphql.String, Description: "Less than"},
			"_lte":      {Type: graphql.String, Description: "Less than or equal to"},
			"_in":       {Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Description: "In the list"},
			"_nin":      {Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Description: "Not in the list"},
			"_is_null":  {Type: graphql.Boolean, Description: "Is null"},
			"_like":     {Type: graphql.String, Description: "LIKE pattern match"},
			"_nlike":    {Type: graphql.String, Description: "NOT LIKE pattern match"},
			"_ilike":    {Type: graphql.String, Description: "Case-insensitive LIKE"},
			"_nilike":   {Type: graphql.String, Description: "Case-insensitive NOT LIKE"},
			"_similar":  {Type: graphql.String, Description: "SIMILAR TO pattern match"},
			"_nsimilar": {Type: graphql.String, Description: "NOT SIMILAR TO pattern match"},
			"_regex":    {Type: graphql.String, Description: "POSIX regex match"},
			"_nregex":   {Type: graphql.String, Description: "NOT POSIX regex match"},
			"_iregex":   {Type: graphql.String, Description: "Case-insensitive POSIX regex"},
			"_niregex":  {Type: graphql.String, Description: "Case-insensitive NOT POSIX regex"},
		},
	})

	// Int comparison
	fg.filterTypes["Int_comparison_exp"] = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "Int_comparison_exp",
		Fields: graphql.InputObjectConfigFieldMap{
			"_eq":      {Type: graphql.Int, Description: "Equal to"},
			"_neq":     {Type: graphql.Int, Description: "Not equal to"},
			"_gt":      {Type: graphql.Int, Description: "Greater than"},
			"_gte":     {Type: graphql.Int, Description: "Greater than or equal to"},
			"_lt":      {Type: graphql.Int, Description: "Less than"},
			"_lte":     {Type: graphql.Int, Description: "Less than or equal to"},
			"_in":      {Type: graphql.NewList(graphql.NewNonNull(graphql.Int)), Description: "In the list"},
			"_nin":     {Type: graphql.NewList(graphql.NewNonNull(graphql.Int)), Description: "Not in the list"},
			"_is_null": {Type: graphql.Boolean, Description: "Is null"},
		},
	})

	// Float comparison
	fg.filterTypes["Float_comparison_exp"] = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "Float_comparison_exp",
		Fields: graphql.InputObjectConfigFieldMap{
			"_eq":      {Type: graphql.Float, Description: "Equal to"},
			"_neq":     {Type: graphql.Float, Description: "Not equal to"},
			"_gt":      {Type: graphql.Float, Description: "Greater than"},
			"_gte":     {Type: graphql.Float, Description: "Greater than or equal to"},
			"_lt":      {Type: graphql.Float, Description: "Less than"},
			"_lte":     {Type: graphql.Float, Description: "Less than or equal to"},
			"_in":      {Type: graphql.NewList(graphql.NewNonNull(graphql.Float)), Description: "In the list"},
			"_nin":     {Type: graphql.NewList(graphql.NewNonNull(graphql.Float)), Description: "Not in the list"},
			"_is_null": {Type: graphql.Boolean, Description: "Is null"},
		},
	})

	// Boolean comparison
	fg.filterTypes["Boolean_comparison_exp"] = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "Boolean_comparison_exp",
		Fields: graphql.InputObjectConfigFieldMap{
			"_eq":      {Type: graphql.Boolean, Description: "Equal to"},
			"_neq":     {Type: graphql.Boolean, Description: "Not equal to"},
			"_is_null": {Type: graphql.Boolean, Description: "Is null"},
		},
	})

	// BigInt comparison
	fg.filterTypes["bigint_comparison_exp"] = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "bigint_comparison_exp",
		Fields: graphql.InputObjectConfigFieldMap{
			"_eq":      {Type: BigInt, Description: "Equal to"},
			"_neq":     {Type: BigInt, Description: "Not equal to"},
			"_gt":      {Type: BigInt, Description: "Greater than"},
			"_gte":     {Type: BigInt, Description: "Greater than or equal to"},
			"_lt":      {Type: BigInt, Description: "Less than"},
			"_lte":     {Type: BigInt, Description: "Less than or equal to"},
			"_in":      {Type: graphql.NewList(graphql.NewNonNull(BigInt)), Description: "In the list"},
			"_nin":     {Type: graphql.NewList(graphql.NewNonNull(BigInt)), Description: "Not in the list"},
			"_is_null": {Type: graphql.Boolean, Description: "Is null"},
		},
	})

	// UUID comparison
	fg.filterTypes["uuid_comparison_exp"] = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "uuid_comparison_exp",
		Fields: graphql.InputObjectConfigFieldMap{
			"_eq":      {Type: UUID, Description: "Equal to"},
			"_neq":     {Type: UUID, Description: "Not equal to"},
			"_gt":      {Type: UUID, Description: "Greater than"},
			"_gte":     {Type: UUID, Description: "Greater than or equal to"},
			"_lt":      {Type: UUID, Description: "Less than"},
			"_lte":     {Type: UUID, Description: "Less than or equal to"},
			"_in":      {Type: graphql.NewList(graphql.NewNonNull(UUID)), Description: "In the list"},
			"_nin":     {Type: graphql.NewList(graphql.NewNonNull(UUID)), Description: "Not in the list"},
			"_is_null": {Type: graphql.Boolean, Description: "Is null"},
		},
	})

	// Timestamp comparison
	fg.filterTypes["timestamp_comparison_exp"] = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "timestamp_comparison_exp",
		Fields: graphql.InputObjectConfigFieldMap{
			"_eq":      {Type: Timestamp, Description: "Equal to"},
			"_neq":     {Type: Timestamp, Description: "Not equal to"},
			"_gt":      {Type: Timestamp, Description: "Greater than"},
			"_gte":     {Type: Timestamp, Description: "Greater than or equal to"},
			"_lt":      {Type: Timestamp, Description: "Less than"},
			"_lte":     {Type: Timestamp, Description: "Less than or equal to"},
			"_in":      {Type: graphql.NewList(graphql.NewNonNull(Timestamp)), Description: "In the list"},
			"_nin":     {Type: graphql.NewList(graphql.NewNonNull(Timestamp)), Description: "Not in the list"},
			"_is_null": {Type: graphql.Boolean, Description: "Is null"},
		},
	})

	// Date comparison
	fg.filterTypes["date_comparison_exp"] = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "date_comparison_exp",
		Fields: graphql.InputObjectConfigFieldMap{
			"_eq":      {Type: Date, Description: "Equal to"},
			"_neq":     {Type: Date, Description: "Not equal to"},
			"_gt":      {Type: Date, Description: "Greater than"},
			"_gte":     {Type: Date, Description: "Greater than or equal to"},
			"_lt":      {Type: Date, Description: "Less than"},
			"_lte":     {Type: Date, Description: "Less than or equal to"},
			"_in":      {Type: graphql.NewList(graphql.NewNonNull(Date)), Description: "In the list"},
			"_nin":     {Type: graphql.NewList(graphql.NewNonNull(Date)), Description: "Not in the list"},
			"_is_null": {Type: graphql.Boolean, Description: "Is null"},
		},
	})

	// JSONB comparison
	fg.filterTypes["jsonb_comparison_exp"] = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "jsonb_comparison_exp",
		Fields: graphql.InputObjectConfigFieldMap{
			"_eq":           {Type: JSON, Description: "Equal to"},
			"_neq":          {Type: JSON, Description: "Not equal to"},
			"_is_null":      {Type: graphql.Boolean, Description: "Is null"},
			"_contains":     {Type: JSON, Description: "Contains"},
			"_contained_in": {Type: JSON, Description: "Contained in"},
			"_has_key":      {Type: graphql.String, Description: "Has key"},
			"_has_keys_any": {Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Description: "Has any keys"},
			"_has_keys_all": {Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Description: "Has all keys"},
		},
	})

	// Generate comparison types for enums
	for enumName, enumType := range fg.enumTypes {
		if enumName == "order_by" || enumName == "cursor_ordering" {
			continue // Skip common enums
		}

		fg.filterTypes[enumName+"_comparison_exp"] = graphql.NewInputObject(graphql.InputObjectConfig{
			Name: toPascalCase(enumName) + "_comparison_exp",
			Fields: graphql.InputObjectConfigFieldMap{
				"_eq":      {Type: enumType, Description: "Equal to"},
				"_neq":     {Type: enumType, Description: "Not equal to"},
				"_in":      {Type: graphql.NewList(graphql.NewNonNull(enumType)), Description: "In the list"},
				"_nin":     {Type: graphql.NewList(graphql.NewNonNull(enumType)), Description: "Not in the list"},
				"_is_null": {Type: graphql.Boolean, Description: "Is null"},
			},
		})
	}
}

// generateBoolExp creates a bool_exp input type for a table
func (fg *FilterGenerator) generateBoolExp(tableName string) *graphql.InputObject {
	name := toPascalCase(tableName) + "_bool_exp"

	if existing, ok := fg.filterTypes[name]; ok {
		return existing
	}

	table := fg.dbSchema.Tables[tableName]
	if table == nil {
		return nil
	}

	// Create placeholder to handle circular references
	boolExp := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   name,
		Fields: graphql.InputObjectConfigFieldMap{},
	})
	fg.filterTypes[name] = boolExp

	// Build fields
	fields := graphql.InputObjectConfigFieldMap{
		"_and": {
			Type:        graphql.NewList(boolExp),
			Description: "Logical AND",
		},
		"_or": {
			Type:        graphql.NewList(boolExp),
			Description: "Logical OR",
		},
		"_not": {
			Type:        boolExp,
			Description: "Logical NOT",
		},
	}

	// Add column filters
	for _, col := range table.Columns {
		comparisonType := fg.getComparisonType(col)
		if comparisonType != nil {
			fields[col.Name] = &graphql.InputObjectFieldConfig{
				Type:        comparisonType,
				Description: "Filter by " + col.Name,
			}
		}
	}

	// Update the bool_exp with fields
	for fieldName, fieldConfig := range fields {
		boolExp.AddFieldConfig(fieldName, fieldConfig)
	}

	return boolExp
}

// generateBoolExpForView creates a bool_exp input type for a view
func (fg *FilterGenerator) generateBoolExpForView(viewName string) *graphql.InputObject {
	name := toPascalCase(viewName) + "_bool_exp"

	if existing, ok := fg.filterTypes[name]; ok {
		return existing
	}

	view := fg.dbSchema.Views[viewName]
	if view == nil {
		return nil
	}

	// Create placeholder
	boolExp := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   name,
		Fields: graphql.InputObjectConfigFieldMap{},
	})
	fg.filterTypes[name] = boolExp

	// Build fields
	fields := graphql.InputObjectConfigFieldMap{
		"_and": {
			Type:        graphql.NewList(boolExp),
			Description: "Logical AND",
		},
		"_or": {
			Type:        graphql.NewList(boolExp),
			Description: "Logical OR",
		},
		"_not": {
			Type:        boolExp,
			Description: "Logical NOT",
		},
	}

	// Add column filters
	for _, col := range view.Columns {
		comparisonType := fg.getComparisonType(col)
		if comparisonType != nil {
			fields[col.Name] = &graphql.InputObjectFieldConfig{
				Type:        comparisonType,
				Description: "Filter by " + col.Name,
			}
		}
	}

	// Update the bool_exp with fields
	for fieldName, fieldConfig := range fields {
		boolExp.AddFieldConfig(fieldName, fieldConfig)
	}

	return boolExp
}

// getComparisonType returns the appropriate comparison input type for a column
func (fg *FilterGenerator) getComparisonType(col *database.Column) *graphql.InputObject {
	// Check if it's an enum type
	if _, ok := fg.enumTypes[col.SQLType]; ok {
		return fg.filterTypes[col.SQLType+"_comparison_exp"]
	}

	// Map SQL types to comparison types
	switch strings.ToLower(col.SQLType) {
	case "int2", "int4", "smallint", "integer", "smallserial", "serial":
		return fg.filterTypes["Int_comparison_exp"]
	case "int8", "bigint", "bigserial":
		return fg.filterTypes["bigint_comparison_exp"]
	case "float4", "float8", "real", "double precision", "numeric", "decimal", "money":
		return fg.filterTypes["Float_comparison_exp"]
	case "bool", "boolean":
		return fg.filterTypes["Boolean_comparison_exp"]
	case "uuid":
		return fg.filterTypes["uuid_comparison_exp"]
	case "json", "jsonb":
		return fg.filterTypes["jsonb_comparison_exp"]
	case "date":
		return fg.filterTypes["date_comparison_exp"]
	case "timestamp", "timestamptz":
		return fg.filterTypes["timestamp_comparison_exp"]
	default:
		return fg.filterTypes["String_comparison_exp"]
	}
}
