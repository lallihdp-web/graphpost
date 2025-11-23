package schema

import (
	"fmt"
	"strings"

	"github.com/graphpost/graphpost/internal/database"
	"github.com/graphql-go/graphql"
)

// Generator generates GraphQL schema from database schema
type Generator struct {
	dbSchema     *database.Schema
	types        map[string]*graphql.Object
	inputTypes   map[string]*graphql.InputObject
	enumTypes    map[string]*graphql.Enum
	orderByEnums map[string]*graphql.Enum
	relationships map[string][]*Relationship
}

// Relationship represents a relationship between tables
type Relationship struct {
	Name           string
	Type           RelationshipType
	SourceTable    string
	SourceColumn   string
	TargetTable    string
	TargetColumn   string
	IsArray        bool
	ForeignKeyName string
}

// RelationshipType represents the type of relationship
type RelationshipType string

const (
	RelationshipTypeObject RelationshipType = "object"
	RelationshipTypeArray  RelationshipType = "array"
)

// NewGenerator creates a new schema generator
func NewGenerator(dbSchema *database.Schema) *Generator {
	return &Generator{
		dbSchema:      dbSchema,
		types:         make(map[string]*graphql.Object),
		inputTypes:    make(map[string]*graphql.InputObject),
		enumTypes:     make(map[string]*graphql.Enum),
		orderByEnums:  make(map[string]*graphql.Enum),
		relationships: make(map[string][]*Relationship),
	}
}

// Generate creates the complete GraphQL schema
func (g *Generator) Generate() (*graphql.Schema, error) {
	// Step 1: Generate enum types from database enums
	g.generateEnumTypes()

	// Step 2: Generate common enums (order_by, etc.)
	g.generateCommonEnums()

	// Step 3: Build relationships from foreign keys
	g.buildRelationships()

	// Step 4: Generate GraphQL types for each table
	if err := g.generateTableTypes(); err != nil {
		return nil, err
	}

	// Step 5: Generate input types for mutations
	g.generateInputTypes()

	// Step 6: Generate query fields
	queryFields := g.generateQueryFields()

	// Step 7: Generate mutation fields
	mutationFields := g.generateMutationFields()

	// Step 8: Generate subscription fields
	subscriptionFields := g.generateSubscriptionFields()

	// Create root query type
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Query",
		Fields: queryFields,
	})

	// Create root mutation type
	mutationType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Mutation",
		Fields: mutationFields,
	})

	// Create root subscription type
	subscriptionType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Subscription",
		Fields: subscriptionFields,
	})

	// Create schema
	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query:        queryType,
		Mutation:     mutationType,
		Subscription: subscriptionType,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &schema, nil
}

// generateEnumTypes creates GraphQL enum types from database enums
func (g *Generator) generateEnumTypes() {
	for name, enum := range g.dbSchema.Enums {
		enumValues := graphql.EnumValueConfigMap{}
		for _, value := range enum.Values {
			enumValues[value] = &graphql.EnumValueConfig{
				Value: value,
			}
		}

		g.enumTypes[name] = graphql.NewEnum(graphql.EnumConfig{
			Name:   toPascalCase(name),
			Values: enumValues,
		})
	}
}

// generateCommonEnums creates common GraphQL enums
func (g *Generator) generateCommonEnums() {
	// Order by enum
	g.enumTypes["order_by"] = graphql.NewEnum(graphql.EnumConfig{
		Name: "order_by",
		Values: graphql.EnumValueConfigMap{
			"asc": &graphql.EnumValueConfig{
				Value:       "ASC",
				Description: "Sort ascending",
			},
			"asc_nulls_first": &graphql.EnumValueConfig{
				Value:       "ASC NULLS FIRST",
				Description: "Sort ascending with nulls first",
			},
			"asc_nulls_last": &graphql.EnumValueConfig{
				Value:       "ASC NULLS LAST",
				Description: "Sort ascending with nulls last",
			},
			"desc": &graphql.EnumValueConfig{
				Value:       "DESC",
				Description: "Sort descending",
			},
			"desc_nulls_first": &graphql.EnumValueConfig{
				Value:       "DESC NULLS FIRST",
				Description: "Sort descending with nulls first",
			},
			"desc_nulls_last": &graphql.EnumValueConfig{
				Value:       "DESC NULLS LAST",
				Description: "Sort descending with nulls last",
			},
		},
	})

	// Cursor ordering enum
	g.enumTypes["cursor_ordering"] = graphql.NewEnum(graphql.EnumConfig{
		Name: "cursor_ordering",
		Values: graphql.EnumValueConfigMap{
			"ASC": &graphql.EnumValueConfig{
				Value:       "ASC",
				Description: "Ascending ordering of the cursor",
			},
			"DESC": &graphql.EnumValueConfig{
				Value:       "DESC",
				Description: "Descending ordering of the cursor",
			},
		},
	})
}

// buildRelationships builds relationships from foreign keys
func (g *Generator) buildRelationships() {
	for _, fk := range g.dbSchema.ForeignKeys {
		// Object relationship (many-to-one): source table can access target table
		objectRel := &Relationship{
			Name:           g.generateRelationshipName(fk.TargetTable, fk.SourceColumns[0]),
			Type:           RelationshipTypeObject,
			SourceTable:    fk.SourceTable,
			SourceColumn:   fk.SourceColumns[0],
			TargetTable:    fk.TargetTable,
			TargetColumn:   fk.TargetColumns[0],
			IsArray:        false,
			ForeignKeyName: fk.Name,
		}
		g.relationships[fk.SourceTable] = append(g.relationships[fk.SourceTable], objectRel)

		// Array relationship (one-to-many): target table can access source table
		arrayRel := &Relationship{
			Name:           g.generateArrayRelationshipName(fk.SourceTable),
			Type:           RelationshipTypeArray,
			SourceTable:    fk.TargetTable,
			SourceColumn:   fk.TargetColumns[0],
			TargetTable:    fk.SourceTable,
			TargetColumn:   fk.SourceColumns[0],
			IsArray:        true,
			ForeignKeyName: fk.Name,
		}
		g.relationships[fk.TargetTable] = append(g.relationships[fk.TargetTable], arrayRel)
	}
}

// generateRelationshipName generates a name for an object relationship
func (g *Generator) generateRelationshipName(targetTable, sourceColumn string) string {
	// Remove _id suffix if present
	name := strings.TrimSuffix(sourceColumn, "_id")
	if name == sourceColumn {
		return targetTable
	}
	return name
}

// generateArrayRelationshipName generates a name for an array relationship
func (g *Generator) generateArrayRelationshipName(sourceTable string) string {
	return sourceTable + "s"
}

// generateTableTypes creates GraphQL object types for each table
func (g *Generator) generateTableTypes() error {
	// First pass: create empty types to handle circular references
	for tableName := range g.dbSchema.Tables {
		g.types[tableName] = graphql.NewObject(graphql.ObjectConfig{
			Name:   toPascalCase(tableName),
			Fields: graphql.Fields{},
		})
	}

	// Second pass: add fields to types
	for tableName, table := range g.dbSchema.Tables {
		fields := graphql.Fields{}

		// Add column fields
		for _, col := range table.Columns {
			field := g.columnToField(col)
			if field != nil {
				fields[col.Name] = field
			}
		}

		// Add relationship fields (will be resolved later)
		for _, rel := range g.relationships[tableName] {
			if rel.IsArray {
				if targetType, ok := g.types[rel.TargetTable]; ok {
					fields[rel.Name] = &graphql.Field{
						Type:        graphql.NewList(targetType),
						Description: fmt.Sprintf("Array relationship to %s", rel.TargetTable),
						Args: graphql.FieldConfigArgument{
							"where":    {Type: g.getWhereInputType(rel.TargetTable)},
							"order_by": {Type: graphql.NewList(g.getOrderByInputType(rel.TargetTable))},
							"limit":    {Type: graphql.Int},
							"offset":   {Type: graphql.Int},
						},
					}
				}
			} else {
				if targetType, ok := g.types[rel.TargetTable]; ok {
					fields[rel.Name] = &graphql.Field{
						Type:        targetType,
						Description: fmt.Sprintf("Object relationship to %s", rel.TargetTable),
					}
				}
			}
		}

		// Update the type with fields
		for name, field := range fields {
			g.types[tableName].AddFieldConfig(name, field)
		}
	}

	// Also generate types for views
	for viewName, view := range g.dbSchema.Views {
		fields := graphql.Fields{}

		for _, col := range view.Columns {
			field := g.columnToField(col)
			if field != nil {
				fields[col.Name] = field
			}
		}

		g.types[viewName] = graphql.NewObject(graphql.ObjectConfig{
			Name:        toPascalCase(viewName),
			Description: view.Comment,
			Fields:      fields,
		})
	}

	return nil
}

// columnToField converts a database column to a GraphQL field
func (g *Generator) columnToField(col *database.Column) *graphql.Field {
	gqlType := g.sqlTypeToGraphQL(col)
	if gqlType == nil {
		return nil
	}

	if !col.IsNullable {
		gqlType = graphql.NewNonNull(gqlType)
	}

	return &graphql.Field{
		Type:        gqlType,
		Description: col.Comment,
	}
}

// sqlTypeToGraphQL converts SQL type to GraphQL type
func (g *Generator) sqlTypeToGraphQL(col *database.Column) graphql.Output {
	var baseType graphql.Output

	// Check if it's an enum type
	if enumType, ok := g.enumTypes[col.SQLType]; ok {
		baseType = enumType
	} else {
		// Map SQL types to GraphQL types
		switch strings.ToLower(col.SQLType) {
		case "int2", "int4", "smallint", "integer", "smallserial", "serial":
			baseType = graphql.Int
		case "int8", "bigint", "bigserial":
			baseType = BigInt
		case "float4", "float8", "real", "double precision", "numeric", "decimal", "money":
			baseType = graphql.Float
		case "bool", "boolean":
			baseType = graphql.Boolean
		case "uuid":
			baseType = UUID
		case "json", "jsonb":
			baseType = JSON
		case "date":
			baseType = Date
		case "time", "timetz":
			baseType = Time
		case "timestamp", "timestamptz":
			baseType = Timestamp
		case "interval":
			baseType = graphql.String
		case "bytea":
			baseType = graphql.String // Base64 encoded
		case "inet", "cidr", "macaddr":
			baseType = graphql.String
		case "point", "line", "lseg", "box", "path", "polygon", "circle":
			baseType = JSON // Geometry types as JSON
		case "tsvector", "tsquery":
			baseType = graphql.String
		default:
			baseType = graphql.String
		}
	}

	// Handle array types
	if col.IsArray {
		return graphql.NewList(baseType)
	}

	return baseType
}

// generateInputTypes creates input types for mutations
func (g *Generator) generateInputTypes() {
	for tableName, table := range g.dbSchema.Tables {
		// Insert input type
		insertFields := graphql.InputObjectConfigFieldMap{}
		for _, col := range table.Columns {
			inputType := g.sqlTypeToGraphQLInput(col)
			if inputType != nil {
				// For insert, only PK columns with defaults can be nullable
				if !col.HasDefault && col.IsPrimaryKey {
					inputType = graphql.NewNonNull(inputType)
				}
				insertFields[col.Name] = &graphql.InputObjectFieldConfig{
					Type:        inputType,
					Description: col.Comment,
				}
			}
		}

		g.inputTypes[tableName+"_insert_input"] = graphql.NewInputObject(graphql.InputObjectConfig{
			Name:   toPascalCase(tableName) + "_insert_input",
			Fields: insertFields,
		})

		// Update input type (all fields optional)
		updateFields := graphql.InputObjectConfigFieldMap{}
		for _, col := range table.Columns {
			inputType := g.sqlTypeToGraphQLInput(col)
			if inputType != nil {
				updateFields[col.Name] = &graphql.InputObjectFieldConfig{
					Type:        inputType,
					Description: col.Comment,
				}
			}
		}

		// Add _inc, _set, _append, _prepend, _delete_key, _delete_elem for specific types
		g.inputTypes[tableName+"_set_input"] = graphql.NewInputObject(graphql.InputObjectConfig{
			Name:   toPascalCase(tableName) + "_set_input",
			Fields: updateFields,
		})

		// Inc input for numeric columns
		incFields := graphql.InputObjectConfigFieldMap{}
		for _, col := range table.Columns {
			if g.isNumericType(col.SQLType) {
				incFields[col.Name] = &graphql.InputObjectFieldConfig{
					Type: graphql.Int,
				}
			}
		}
		if len(incFields) > 0 {
			g.inputTypes[tableName+"_inc_input"] = graphql.NewInputObject(graphql.InputObjectConfig{
				Name:   toPascalCase(tableName) + "_inc_input",
				Fields: incFields,
			})
		}

		// Primary key input
		pkFields := graphql.InputObjectConfigFieldMap{}
		if table.PrimaryKey != nil {
			for _, col := range table.Columns {
				if col.IsPrimaryKey {
					inputType := g.sqlTypeToGraphQLInput(col)
					if inputType != nil {
						pkFields[col.Name] = &graphql.InputObjectFieldConfig{
							Type: graphql.NewNonNull(inputType),
						}
					}
				}
			}
		}
		if len(pkFields) > 0 {
			g.inputTypes[tableName+"_pk_columns_input"] = graphql.NewInputObject(graphql.InputObjectConfig{
				Name:   toPascalCase(tableName) + "_pk_columns_input",
				Fields: pkFields,
			})
		}
	}
}

// sqlTypeToGraphQLInput converts SQL type to GraphQL input type
func (g *Generator) sqlTypeToGraphQLInput(col *database.Column) graphql.Input {
	var baseType graphql.Input

	// Check if it's an enum type
	if enumType, ok := g.enumTypes[col.SQLType]; ok {
		baseType = enumType
	} else {
		switch strings.ToLower(col.SQLType) {
		case "int2", "int4", "smallint", "integer", "smallserial", "serial":
			baseType = graphql.Int
		case "int8", "bigint", "bigserial":
			baseType = BigInt
		case "float4", "float8", "real", "double precision", "numeric", "decimal", "money":
			baseType = graphql.Float
		case "bool", "boolean":
			baseType = graphql.Boolean
		case "uuid":
			baseType = UUID
		case "json", "jsonb":
			baseType = JSON
		case "date":
			baseType = Date
		case "time", "timetz":
			baseType = Time
		case "timestamp", "timestamptz":
			baseType = Timestamp
		default:
			baseType = graphql.String
		}
	}

	if col.IsArray {
		return graphql.NewList(baseType)
	}

	return baseType
}

// getWhereInputType returns the where input type for a table
func (g *Generator) getWhereInputType(tableName string) *graphql.InputObject {
	name := toPascalCase(tableName) + "_bool_exp"

	if inputType, ok := g.inputTypes[name]; ok {
		return inputType
	}

	// Create a placeholder - this will be properly filled in later
	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   name,
		Fields: graphql.InputObjectConfigFieldMap{},
	})
}

// getOrderByInputType returns the order by input type for a table
func (g *Generator) getOrderByInputType(tableName string) *graphql.InputObject {
	name := toPascalCase(tableName) + "_order_by"

	if inputType, ok := g.inputTypes[name]; ok {
		return inputType
	}

	// Create order by input with all columns
	fields := graphql.InputObjectConfigFieldMap{}
	if table, ok := g.dbSchema.Tables[tableName]; ok {
		for _, col := range table.Columns {
			fields[col.Name] = &graphql.InputObjectFieldConfig{
				Type: g.enumTypes["order_by"],
			}
		}
	}

	inputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   name,
		Fields: fields,
	})
	g.inputTypes[name] = inputType
	return inputType
}

// generateQueryFields creates query fields for the schema
func (g *Generator) generateQueryFields() graphql.Fields {
	fields := graphql.Fields{}

	for tableName := range g.dbSchema.Tables {
		tableType := g.types[tableName]
		if tableType == nil {
			continue
		}

		// Query for multiple records: tableName(where, order_by, limit, offset)
		fields[tableName] = &graphql.Field{
			Type:        graphql.NewList(tableType),
			Description: fmt.Sprintf("Fetch data from table: %s", tableName),
			Args: graphql.FieldConfigArgument{
				"where": {
					Type:        g.getWhereInputType(tableName),
					Description: "Filter the results",
				},
				"order_by": {
					Type:        graphql.NewList(g.getOrderByInputType(tableName)),
					Description: "Sort the results",
				},
				"limit": {
					Type:        graphql.Int,
					Description: "Limit the number of results",
				},
				"offset": {
					Type:        graphql.Int,
					Description: "Skip the first n results",
				},
				"distinct_on": {
					Type:        graphql.NewList(graphql.String),
					Description: "Select distinct on columns",
				},
			},
		}

		// Query by primary key: tableName_by_pk(pk_columns)
		if table, ok := g.dbSchema.Tables[tableName]; ok && table.PrimaryKey != nil {
			pkArgs := graphql.FieldConfigArgument{}
			for _, col := range table.Columns {
				if col.IsPrimaryKey {
					gqlType := g.sqlTypeToGraphQL(col)
					if gqlType != nil {
						pkArgs[col.Name] = &graphql.ArgumentConfig{
							Type: graphql.NewNonNull(gqlType),
						}
					}
				}
			}

			if len(pkArgs) > 0 {
				fields[tableName+"_by_pk"] = &graphql.Field{
					Type:        tableType,
					Description: fmt.Sprintf("Fetch a single row from table %s by primary key", tableName),
					Args:        pkArgs,
				}
			}
		}

		// Aggregate query: tableName_aggregate(where, order_by, limit, offset)
		fields[tableName+"_aggregate"] = &graphql.Field{
			Type:        g.getAggregateType(tableName),
			Description: fmt.Sprintf("Aggregate data from table: %s", tableName),
			Args: graphql.FieldConfigArgument{
				"where": {
					Type:        g.getWhereInputType(tableName),
					Description: "Filter the results",
				},
				"order_by": {
					Type:        graphql.NewList(g.getOrderByInputType(tableName)),
					Description: "Sort the results",
				},
				"limit": {
					Type:        graphql.Int,
					Description: "Limit the number of results",
				},
				"offset": {
					Type:        graphql.Int,
					Description: "Skip the first n results",
				},
			},
		}
	}

	// Add view queries
	for viewName := range g.dbSchema.Views {
		viewType := g.types[viewName]
		if viewType == nil {
			continue
		}

		fields[viewName] = &graphql.Field{
			Type:        graphql.NewList(viewType),
			Description: fmt.Sprintf("Fetch data from view: %s", viewName),
			Args: graphql.FieldConfigArgument{
				"where": {
					Type:        g.getWhereInputType(viewName),
					Description: "Filter the results",
				},
				"order_by": {
					Type:        graphql.NewList(g.getOrderByInputType(viewName)),
					Description: "Sort the results",
				},
				"limit": {
					Type:        graphql.Int,
					Description: "Limit the number of results",
				},
				"offset": {
					Type:        graphql.Int,
					Description: "Skip the first n results",
				},
			},
		}
	}

	return fields
}

// generateMutationFields creates mutation fields for the schema
func (g *Generator) generateMutationFields() graphql.Fields {
	fields := graphql.Fields{}

	for tableName := range g.dbSchema.Tables {
		tableType := g.types[tableName]
		if tableType == nil {
			continue
		}

		insertInput := g.inputTypes[tableName+"_insert_input"]
		setInput := g.inputTypes[tableName+"_set_input"]
		pkInput := g.inputTypes[tableName+"_pk_columns_input"]

		// Insert single: insert_tableName_one(object)
		if insertInput != nil {
			fields["insert_"+tableName+"_one"] = &graphql.Field{
				Type:        tableType,
				Description: fmt.Sprintf("Insert a single row into table: %s", tableName),
				Args: graphql.FieldConfigArgument{
					"object": {
						Type:        graphql.NewNonNull(insertInput),
						Description: "The row to insert",
					},
					"on_conflict": {
						Type:        g.getOnConflictInputType(tableName),
						Description: "On conflict condition",
					},
				},
			}

			// Insert multiple: insert_tableName(objects)
			fields["insert_"+tableName] = &graphql.Field{
				Type:        g.getMutationResponseType(tableName),
				Description: fmt.Sprintf("Insert multiple rows into table: %s", tableName),
				Args: graphql.FieldConfigArgument{
					"objects": {
						Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(insertInput))),
						Description: "The rows to insert",
					},
					"on_conflict": {
						Type:        g.getOnConflictInputType(tableName),
						Description: "On conflict condition",
					},
				},
			}
		}

		// Update by primary key: update_tableName_by_pk(pk_columns, _set)
		if pkInput != nil && setInput != nil {
			fields["update_"+tableName+"_by_pk"] = &graphql.Field{
				Type:        tableType,
				Description: fmt.Sprintf("Update a single row in table %s by primary key", tableName),
				Args: graphql.FieldConfigArgument{
					"pk_columns": {
						Type:        graphql.NewNonNull(pkInput),
						Description: "Primary key columns",
					},
					"_set": {
						Type:        setInput,
						Description: "Sets the columns to the given values",
					},
					"_inc": {
						Type:        g.inputTypes[tableName+"_inc_input"],
						Description: "Increments the numeric columns",
					},
				},
			}
		}

		// Update multiple: update_tableName(where, _set)
		if setInput != nil {
			fields["update_"+tableName] = &graphql.Field{
				Type:        g.getMutationResponseType(tableName),
				Description: fmt.Sprintf("Update rows in table: %s", tableName),
				Args: graphql.FieldConfigArgument{
					"where": {
						Type:        graphql.NewNonNull(g.getWhereInputType(tableName)),
						Description: "Filter the rows to update",
					},
					"_set": {
						Type:        setInput,
						Description: "Sets the columns to the given values",
					},
					"_inc": {
						Type:        g.inputTypes[tableName+"_inc_input"],
						Description: "Increments the numeric columns",
					},
				},
			}
		}

		// Delete by primary key: delete_tableName_by_pk(pk_columns)
		if pkInput != nil {
			pkArgs := graphql.FieldConfigArgument{}
			if table, ok := g.dbSchema.Tables[tableName]; ok && table.PrimaryKey != nil {
				for _, col := range table.Columns {
					if col.IsPrimaryKey {
						gqlType := g.sqlTypeToGraphQL(col)
						if gqlType != nil {
							pkArgs[col.Name] = &graphql.ArgumentConfig{
								Type: graphql.NewNonNull(gqlType),
							}
						}
					}
				}
			}

			if len(pkArgs) > 0 {
				fields["delete_"+tableName+"_by_pk"] = &graphql.Field{
					Type:        tableType,
					Description: fmt.Sprintf("Delete a single row from table %s by primary key", tableName),
					Args:        pkArgs,
				}
			}
		}

		// Delete multiple: delete_tableName(where)
		fields["delete_"+tableName] = &graphql.Field{
			Type:        g.getMutationResponseType(tableName),
			Description: fmt.Sprintf("Delete rows from table: %s", tableName),
			Args: graphql.FieldConfigArgument{
				"where": {
					Type:        graphql.NewNonNull(g.getWhereInputType(tableName)),
					Description: "Filter the rows to delete",
				},
			},
		}
	}

	return fields
}

// generateSubscriptionFields creates subscription fields for the schema
func (g *Generator) generateSubscriptionFields() graphql.Fields {
	fields := graphql.Fields{}

	for tableName := range g.dbSchema.Tables {
		tableType := g.types[tableName]
		if tableType == nil {
			continue
		}

		// Subscribe to table changes: tableName(where, order_by, limit, offset)
		fields[tableName] = &graphql.Field{
			Type:        graphql.NewList(tableType),
			Description: fmt.Sprintf("Subscribe to changes in table: %s", tableName),
			Args: graphql.FieldConfigArgument{
				"where": {
					Type:        g.getWhereInputType(tableName),
					Description: "Filter the results",
				},
				"order_by": {
					Type:        graphql.NewList(g.getOrderByInputType(tableName)),
					Description: "Sort the results",
				},
				"limit": {
					Type:        graphql.Int,
					Description: "Limit the number of results",
				},
				"offset": {
					Type:        graphql.Int,
					Description: "Skip the first n results",
				},
			},
		}

		// Subscribe by primary key: tableName_by_pk(pk_columns)
		if table, ok := g.dbSchema.Tables[tableName]; ok && table.PrimaryKey != nil {
			pkArgs := graphql.FieldConfigArgument{}
			for _, col := range table.Columns {
				if col.IsPrimaryKey {
					gqlType := g.sqlTypeToGraphQL(col)
					if gqlType != nil {
						pkArgs[col.Name] = &graphql.ArgumentConfig{
							Type: graphql.NewNonNull(gqlType),
						}
					}
				}
			}

			if len(pkArgs) > 0 {
				fields[tableName+"_by_pk"] = &graphql.Field{
					Type:        tableType,
					Description: fmt.Sprintf("Subscribe to a single row in table %s by primary key", tableName),
					Args:        pkArgs,
				}
			}
		}

		// Stream subscription: tableName_stream(where, cursor, batch_size)
		fields[tableName+"_stream"] = &graphql.Field{
			Type:        graphql.NewList(tableType),
			Description: fmt.Sprintf("Stream changes from table: %s", tableName),
			Args: graphql.FieldConfigArgument{
				"where": {
					Type:        g.getWhereInputType(tableName),
					Description: "Filter the results",
				},
				"cursor": {
					Type:        graphql.NewList(g.getStreamCursorInputType(tableName)),
					Description: "Cursor for streaming",
				},
				"batch_size": {
					Type:        graphql.NewNonNull(graphql.Int),
					Description: "Maximum number of rows to return per batch",
				},
			},
		}
	}

	return fields
}

// getAggregateType returns the aggregate type for a table
func (g *Generator) getAggregateType(tableName string) *graphql.Object {
	name := toPascalCase(tableName) + "_aggregate"

	// Aggregate fields type
	aggFieldsName := toPascalCase(tableName) + "_aggregate_fields"
	aggFields := graphql.NewObject(graphql.ObjectConfig{
		Name: aggFieldsName,
		Fields: graphql.Fields{
			"count": {
				Type: graphql.Int,
				Args: graphql.FieldConfigArgument{
					"columns": {
						Type: graphql.NewList(graphql.String),
					},
					"distinct": {
						Type: graphql.Boolean,
					},
				},
			},
			"max": {Type: g.getMinMaxType(tableName, "max")},
			"min": {Type: g.getMinMaxType(tableName, "min")},
			"sum": {Type: g.getSumType(tableName)},
			"avg": {Type: g.getAvgType(tableName)},
			"stddev": {Type: g.getStddevType(tableName)},
			"stddev_pop": {Type: g.getStddevType(tableName)},
			"stddev_samp": {Type: g.getStddevType(tableName)},
			"var_pop": {Type: g.getVarianceType(tableName)},
			"var_samp": {Type: g.getVarianceType(tableName)},
			"variance": {Type: g.getVarianceType(tableName)},
		},
	})

	return graphql.NewObject(graphql.ObjectConfig{
		Name: name,
		Fields: graphql.Fields{
			"aggregate": {
				Type: aggFields,
			},
			"nodes": {
				Type: graphql.NewList(g.types[tableName]),
			},
		},
	})
}

// getMinMaxType returns the min/max type for a table
func (g *Generator) getMinMaxType(tableName, prefix string) *graphql.Object {
	name := toPascalCase(tableName) + "_" + prefix + "_fields"
	fields := graphql.Fields{}

	if table, ok := g.dbSchema.Tables[tableName]; ok {
		for _, col := range table.Columns {
			if g.isComparableType(col.SQLType) {
				fields[col.Name] = &graphql.Field{
					Type: g.sqlTypeToGraphQL(col),
				}
			}
		}
	}

	return graphql.NewObject(graphql.ObjectConfig{
		Name:   name,
		Fields: fields,
	})
}

// getSumType returns the sum type for a table
func (g *Generator) getSumType(tableName string) *graphql.Object {
	name := toPascalCase(tableName) + "_sum_fields"
	fields := graphql.Fields{}

	if table, ok := g.dbSchema.Tables[tableName]; ok {
		for _, col := range table.Columns {
			if g.isNumericType(col.SQLType) {
				fields[col.Name] = &graphql.Field{
					Type: graphql.Float,
				}
			}
		}
	}

	return graphql.NewObject(graphql.ObjectConfig{
		Name:   name,
		Fields: fields,
	})
}

// getAvgType returns the avg type for a table
func (g *Generator) getAvgType(tableName string) *graphql.Object {
	name := toPascalCase(tableName) + "_avg_fields"
	fields := graphql.Fields{}

	if table, ok := g.dbSchema.Tables[tableName]; ok {
		for _, col := range table.Columns {
			if g.isNumericType(col.SQLType) {
				fields[col.Name] = &graphql.Field{
					Type: graphql.Float,
				}
			}
		}
	}

	return graphql.NewObject(graphql.ObjectConfig{
		Name:   name,
		Fields: fields,
	})
}

// getStddevType returns the stddev type for a table
func (g *Generator) getStddevType(tableName string) *graphql.Object {
	name := toPascalCase(tableName) + "_stddev_fields"
	fields := graphql.Fields{}

	if table, ok := g.dbSchema.Tables[tableName]; ok {
		for _, col := range table.Columns {
			if g.isNumericType(col.SQLType) {
				fields[col.Name] = &graphql.Field{
					Type: graphql.Float,
				}
			}
		}
	}

	return graphql.NewObject(graphql.ObjectConfig{
		Name:   name,
		Fields: fields,
	})
}

// getVarianceType returns the variance type for a table
func (g *Generator) getVarianceType(tableName string) *graphql.Object {
	name := toPascalCase(tableName) + "_variance_fields"
	fields := graphql.Fields{}

	if table, ok := g.dbSchema.Tables[tableName]; ok {
		for _, col := range table.Columns {
			if g.isNumericType(col.SQLType) {
				fields[col.Name] = &graphql.Field{
					Type: graphql.Float,
				}
			}
		}
	}

	return graphql.NewObject(graphql.ObjectConfig{
		Name:   name,
		Fields: fields,
	})
}

// getMutationResponseType returns the mutation response type for a table
func (g *Generator) getMutationResponseType(tableName string) *graphql.Object {
	name := toPascalCase(tableName) + "_mutation_response"

	return graphql.NewObject(graphql.ObjectConfig{
		Name: name,
		Fields: graphql.Fields{
			"affected_rows": {
				Type:        graphql.NewNonNull(graphql.Int),
				Description: "Number of affected rows",
			},
			"returning": {
				Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(g.types[tableName]))),
				Description: "Data of affected rows",
			},
		},
	})
}

// getOnConflictInputType returns the on_conflict input type for a table
func (g *Generator) getOnConflictInputType(tableName string) *graphql.InputObject {
	name := toPascalCase(tableName) + "_on_conflict"

	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name: name,
		Fields: graphql.InputObjectConfigFieldMap{
			"constraint": {
				Type:        graphql.String,
				Description: "Constraint name",
			},
			"update_columns": {
				Type:        graphql.NewList(graphql.String),
				Description: "Columns to update on conflict",
			},
			"where": {
				Type:        g.getWhereInputType(tableName),
				Description: "Filter for updating",
			},
		},
	})
}

// getStreamCursorInputType returns the stream cursor input type for a table
func (g *Generator) getStreamCursorInputType(tableName string) *graphql.InputObject {
	name := toPascalCase(tableName) + "_stream_cursor_input"

	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name: name,
		Fields: graphql.InputObjectConfigFieldMap{
			"initial_value": {
				Type:        graphql.NewNonNull(g.getStreamCursorValueInputType(tableName)),
				Description: "Initial cursor value",
			},
			"ordering": {
				Type:        g.enumTypes["cursor_ordering"],
				Description: "Cursor ordering",
			},
		},
	})
}

// getStreamCursorValueInputType returns the stream cursor value input type for a table
func (g *Generator) getStreamCursorValueInputType(tableName string) *graphql.InputObject {
	name := toPascalCase(tableName) + "_stream_cursor_value_input"
	fields := graphql.InputObjectConfigFieldMap{}

	if table, ok := g.dbSchema.Tables[tableName]; ok {
		for _, col := range table.Columns {
			inputType := g.sqlTypeToGraphQLInput(col)
			if inputType != nil {
				fields[col.Name] = &graphql.InputObjectFieldConfig{
					Type: inputType,
				}
			}
		}
	}

	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   name,
		Fields: fields,
	})
}

// isNumericType checks if a SQL type is numeric
func (g *Generator) isNumericType(sqlType string) bool {
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

// isComparableType checks if a SQL type is comparable (for min/max)
func (g *Generator) isComparableType(sqlType string) bool {
	if g.isNumericType(sqlType) {
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

// toPascalCase converts snake_case to PascalCase
func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}
