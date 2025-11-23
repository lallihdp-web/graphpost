package schema

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
)

// Custom scalar types for PostgreSQL data types

// UUID scalar type
var UUID = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "uuid",
	Description: "UUID scalar type",
	Serialize: func(value interface{}) interface{} {
		switch v := value.(type) {
		case string:
			return v
		case []byte:
			return string(v)
		default:
			return nil
		}
	},
	ParseValue: func(value interface{}) interface{} {
		switch v := value.(type) {
		case string:
			return v
		default:
			return nil
		}
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		switch v := valueAST.(type) {
		case *ast.StringValue:
			return v.Value
		default:
			return nil
		}
	},
})

// JSON scalar type for jsonb columns
var JSON = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "jsonb",
	Description: "JSON scalar type for PostgreSQL jsonb columns",
	Serialize: func(value interface{}) interface{} {
		switch v := value.(type) {
		case string:
			var result interface{}
			if err := json.Unmarshal([]byte(v), &result); err != nil {
				return v
			}
			return result
		case []byte:
			var result interface{}
			if err := json.Unmarshal(v, &result); err != nil {
				return string(v)
			}
			return result
		case map[string]interface{}:
			return v
		case []interface{}:
			return v
		default:
			return v
		}
	},
	ParseValue: func(value interface{}) interface{} {
		return value
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		switch v := valueAST.(type) {
		case *ast.StringValue:
			var result interface{}
			if err := json.Unmarshal([]byte(v.Value), &result); err != nil {
				return v.Value
			}
			return result
		case *ast.ObjectValue:
			return parseObjectValue(v)
		case *ast.ListValue:
			return parseListValue(v)
		default:
			return nil
		}
	},
})

// BigInt scalar type for int8/bigint columns
var BigInt = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "bigint",
	Description: "BigInt scalar type for PostgreSQL bigint columns",
	Serialize: func(value interface{}) interface{} {
		switch v := value.(type) {
		case int64:
			return fmt.Sprintf("%d", v)
		case int:
			return fmt.Sprintf("%d", v)
		case float64:
			return fmt.Sprintf("%.0f", v)
		case string:
			return v
		default:
			return nil
		}
	},
	ParseValue: func(value interface{}) interface{} {
		switch v := value.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		case string:
			var i int64
			if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
				return i
			}
			return nil
		default:
			return nil
		}
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		switch v := valueAST.(type) {
		case *ast.IntValue:
			var i int64
			if _, err := fmt.Sscanf(v.Value, "%d", &i); err == nil {
				return i
			}
			return nil
		case *ast.StringValue:
			var i int64
			if _, err := fmt.Sscanf(v.Value, "%d", &i); err == nil {
				return i
			}
			return nil
		default:
			return nil
		}
	},
})

// Date scalar type
var Date = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "date",
	Description: "Date scalar type (YYYY-MM-DD)",
	Serialize: func(value interface{}) interface{} {
		switch v := value.(type) {
		case time.Time:
			return v.Format("2006-01-02")
		case string:
			return v
		default:
			return nil
		}
	},
	ParseValue: func(value interface{}) interface{} {
		switch v := value.(type) {
		case string:
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				return nil
			}
			return t
		default:
			return nil
		}
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		switch v := valueAST.(type) {
		case *ast.StringValue:
			t, err := time.Parse("2006-01-02", v.Value)
			if err != nil {
				return nil
			}
			return t
		default:
			return nil
		}
	},
})

// Time scalar type
var Time = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "time",
	Description: "Time scalar type (HH:MM:SS)",
	Serialize: func(value interface{}) interface{} {
		switch v := value.(type) {
		case time.Time:
			return v.Format("15:04:05")
		case string:
			return v
		default:
			return nil
		}
	},
	ParseValue: func(value interface{}) interface{} {
		switch v := value.(type) {
		case string:
			t, err := time.Parse("15:04:05", v)
			if err != nil {
				return nil
			}
			return t
		default:
			return nil
		}
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		switch v := valueAST.(type) {
		case *ast.StringValue:
			t, err := time.Parse("15:04:05", v.Value)
			if err != nil {
				return nil
			}
			return t
		default:
			return nil
		}
	},
})

// Timestamp scalar type
var Timestamp = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "timestamp",
	Description: "Timestamp scalar type (ISO 8601)",
	Serialize: func(value interface{}) interface{} {
		switch v := value.(type) {
		case time.Time:
			return v.Format(time.RFC3339)
		case string:
			return v
		default:
			return nil
		}
	},
	ParseValue: func(value interface{}) interface{} {
		switch v := value.(type) {
		case string:
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				// Try other formats
				t, err = time.Parse("2006-01-02T15:04:05", v)
				if err != nil {
					t, err = time.Parse("2006-01-02 15:04:05", v)
					if err != nil {
						return nil
					}
				}
			}
			return t
		default:
			return nil
		}
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		switch v := valueAST.(type) {
		case *ast.StringValue:
			t, err := time.Parse(time.RFC3339, v.Value)
			if err != nil {
				t, err = time.Parse("2006-01-02T15:04:05", v.Value)
				if err != nil {
					t, err = time.Parse("2006-01-02 15:04:05", v.Value)
					if err != nil {
						return nil
					}
				}
			}
			return t
		default:
			return nil
		}
	},
})

// Timestamptz scalar type (with timezone)
var Timestamptz = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "timestamptz",
	Description: "Timestamp with timezone scalar type (ISO 8601)",
	Serialize: func(value interface{}) interface{} {
		switch v := value.(type) {
		case time.Time:
			return v.Format(time.RFC3339)
		case string:
			return v
		default:
			return nil
		}
	},
	ParseValue: func(value interface{}) interface{} {
		switch v := value.(type) {
		case string:
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil
			}
			return t
		default:
			return nil
		}
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		switch v := valueAST.(type) {
		case *ast.StringValue:
			t, err := time.Parse(time.RFC3339, v.Value)
			if err != nil {
				return nil
			}
			return t
		default:
			return nil
		}
	},
})

// Numeric scalar type for decimal/numeric columns
var Numeric = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "numeric",
	Description: "Numeric scalar type for PostgreSQL decimal/numeric columns",
	Serialize: func(value interface{}) interface{} {
		switch v := value.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int64:
			return float64(v)
		case int:
			return float64(v)
		case string:
			return v
		default:
			return nil
		}
	},
	ParseValue: func(value interface{}) interface{} {
		switch v := value.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int64:
			return float64(v)
		case int:
			return float64(v)
		case string:
			var f float64
			if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
				return f
			}
			return nil
		default:
			return nil
		}
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		switch v := valueAST.(type) {
		case *ast.FloatValue:
			var f float64
			if _, err := fmt.Sscanf(v.Value, "%f", &f); err == nil {
				return f
			}
			return nil
		case *ast.IntValue:
			var i int64
			if _, err := fmt.Sscanf(v.Value, "%d", &i); err == nil {
				return float64(i)
			}
			return nil
		case *ast.StringValue:
			var f float64
			if _, err := fmt.Sscanf(v.Value, "%f", &f); err == nil {
				return f
			}
			return nil
		default:
			return nil
		}
	},
})

// Helper functions for parsing AST values

func parseObjectValue(v *ast.ObjectValue) map[string]interface{} {
	result := make(map[string]interface{})
	for _, field := range v.Fields {
		result[field.Name.Value] = parseASTValue(field.Value)
	}
	return result
}

func parseListValue(v *ast.ListValue) []interface{} {
	result := make([]interface{}, len(v.Values))
	for i, value := range v.Values {
		result[i] = parseASTValue(value)
	}
	return result
}

func parseASTValue(v ast.Value) interface{} {
	switch val := v.(type) {
	case *ast.StringValue:
		return val.Value
	case *ast.IntValue:
		var i int64
		fmt.Sscanf(val.Value, "%d", &i)
		return i
	case *ast.FloatValue:
		var f float64
		fmt.Sscanf(val.Value, "%f", &f)
		return f
	case *ast.BooleanValue:
		return val.Value
	case *ast.EnumValue:
		return val.Value
	case *ast.ListValue:
		return parseListValue(val)
	case *ast.ObjectValue:
		return parseObjectValue(val)
	default:
		return nil
	}
}
