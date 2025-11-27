# GraphJin Aggregate Queries

GraphJin automatically generates aggregate functionality for all your database tables. Aggregates use a special naming convention where you prefix the aggregate function to the field name.

## Aggregate Functions

GraphJin supports the following aggregate functions:

- `count_<field>` - Count non-null values
- `sum_<field>` - Sum numeric values
- `avg_<field>` - Calculate average
- `min_<field>` - Find minimum value
- `max_<field>` - Find maximum value

## Syntax

### Basic Aggregate Query

```graphql
query {
  products {
    count_id      # Count of products
    avg_price     # Average price
    min_price     # Minimum price
    max_price     # Maximum price
    sum_quantity  # Sum of quantities
  }
}
```

### Aggregates with Filtering

```graphql
query {
  products(where: { price: { greater_than: 100 } }) {
    count_id
    avg_price
    max_price
  }
}
```

### Aggregates with Grouping

GraphJin automatically handles grouping when you combine aggregates with regular fields:

```graphql
query {
  products {
    category      # Group by category
    count_id      # Count per category
    avg_price     # Average price per category
  }
}
```

## Complete Examples

### Example 1: Total Count and Sum

Get total number of orders and total revenue:

```graphql
query {
  orders {
    count_id
    sum_total_amount
  }
}
```

Result:
```json
{
  "data": {
    "orders": {
      "count_id": 1250,
      "sum_total_amount": 45678.90
    }
  }
}
```

### Example 2: Statistics by Category

Get product statistics grouped by category:

```graphql
query {
  products {
    category
    count_id
    avg_price
    min_price
    max_price
  }
}
```

Result:
```json
{
  "data": {
    "products": [
      {
        "category": "Electronics",
        "count_id": 45,
        "avg_price": 299.99,
        "min_price": 49.99,
        "max_price": 999.99
      },
      {
        "category": "Books",
        "count_id": 120,
        "avg_price": 19.99,
        "min_price": 9.99,
        "max_price": 49.99
      }
    ]
  }
}
```

### Example 3: Filtered Aggregates

Get average rating for products with more than 10 reviews:

```graphql
query {
  products(where: { review_count: { greater_than: 10 } }) {
    count_id
    avg_rating
    min_rating
    max_rating
  }
}
```

### Example 4: User Activity Statistics

Count actions per user:

```graphql
query {
  user_actions {
    user_id
    count_id
    action_type
  }
}
```

## Configuration

Aggregates are **enabled by default** in GraphJin. To disable them, set in `config.json`:

```json
{
  "graphjin": {
    "enabled": true,
    "disable_functions": false
  }
}
```

The `DisableAgg` flag in GraphJin's core.Config controls aggregate functions. In GraphPost, this is **hardcoded to false** (aggregates enabled) in `internal/graphjin/engine.go`.

## Important Notes

1. **Field Names**: Use the actual database column names (snake_case) not GraphQL field names (camelCase):
   - ✅ `count_id`
   - ❌ `countId`

2. **Grouping**: When you include both regular fields and aggregates, GraphJin automatically groups by the regular fields.

3. **NULL Handling**: `count_<field>` counts non-null values. Use `count_id` (assuming id is never null) to count all rows.

4. **Performance**: GraphJin compiles these to efficient SQL queries with proper aggregation at the database level.

## SQL Translation Examples

### GraphQL Query:
```graphql
query {
  products {
    category
    count_id
    avg_price
  }
}
```

### Generated SQL:
```sql
SELECT
  category,
  COUNT(id) as count_id,
  AVG(price) as avg_price
FROM products
GROUP BY category
```

## Troubleshooting

### "Field not found" Error

If you get errors about aggregate fields not being found:

1. **Check introspection is enabled**: Set `enable_introspection: true` in config
2. **Verify field names**: Use exact database column names (snake_case)
3. **Check table name**: Use plural form (GraphJin convention) - e.g., `products` not `product`
4. **Restart server**: Schema is cached, restart to reload

### Aggregates Not Available

If aggregate fields don't appear in introspection:

1. Check `disable_functions: false` in `config.json` under `graphjin` section
2. Verify GraphJin is enabled: `"enabled": true`
3. Check server logs for GraphJin initialization message

## Testing Aggregates

To test if aggregates are working, try this simple query:

```graphql
query {
  __type(name: "YourTableName") {
    fields {
      name
    }
  }
}
```

You should see aggregate fields like `count_id`, `avg_fieldname`, etc. in the field list.

## References

- [GraphJin Cheatsheet](https://graphjin.com/posts/cheatsheet)
- [GraphJin GitHub](https://github.com/dosco/graphjin)
- [GraphJin Core Package](https://pkg.go.dev/github.com/dosco/graphjin/core)
- [What does disable_functions mean?](https://github.com/dosco/graphjin/issues/85)
