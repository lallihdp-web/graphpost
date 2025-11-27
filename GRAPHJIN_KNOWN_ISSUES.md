# GraphJin Known Issues and Limitations

## ⚠️ Schema Introspection Doesn't Match Query Capabilities

### The Issue

**GraphJin's introspection schema doesn't properly expose aggregate fields.** This causes:

- ❌ WebUI autocomplete shows errors like "Cannot query field 'count_id'"
- ❌ Schema explorers don't show `count_*`, `sum_*`, `avg_*` fields
- ❌ GraphQL IDEs show validation errors
- ✅ **BUT QUERIES STILL WORK!**

### Example

```graphql
# WebUI shows: "Cannot query field 'count_id' on type 'post_categories'"
# But this query WORKS:
query {
  post_categories {
    category_id
    count_id
    category {
      name
    }
  }
}

# Result: ✅ Success!
{
  "data": {
    "post_categories": [
      {
        "category_id": 3,
        "category": { "name": "Food" },
        "count_id": 1
      }
    ]
  }
}
```

### Why This Happens

GraphJin adds aggregate fields **dynamically during query execution**, not in the static GraphQL schema. The fields are "virtual" - they exist at runtime but not in introspection.

**From GraphJin Discussion #430:**
> GraphJin's introspection schema doesn't strictly conform to GraphQL standards, causing validation failures in strict clients. The schema reflects non-standard behaviors that may confuse strict validators.

## Aggregate Fields That Work (Despite Schema Errors)

### All Tables Support These Fields

Even if the schema doesn't show them, these fields work on **every table**:

| Field Pattern | Description | Example |
|--------------|-------------|---------|
| `count_id` | Count records | `users { count_id }` → 3 |
| `count_<field>` | Count non-null values | `posts { count_title }` |
| `sum_<field>` | Sum numeric field | `orders { sum_total }` |
| `avg_<field>` | Average numeric field | `products { avg_price }` |
| `min_<field>` | Minimum value | `products { min_price }` |
| `max_<field>` | Maximum value | `products { max_price }` |

### Working Queries for Your Schema

Based on `init.sql`, these all work (ignore WebUI errors):

```graphql
# Count users (WebUI shows error, but works!)
query {
  users { count_id }
}

# Count posts per user (WebUI shows error, but works!)
query {
  users {
    name
    posts { count_id }
  }
}

# Posts per category (WebUI shows error, but works!)
query {
  post_categories {
    category_id
    count_id
    category { name }
  }
}

# Aggregate stats (WebUI shows error, but works!)
query {
  posts {
    count_id
    avg_id
    min_id
    max_id
  }
}
```

## How to Use the WebUI Despite Errors

### Step 1: Ignore Red Squiggly Lines

The WebUI will show validation errors:
```
Cannot query field 'count_id' on type 'categories'
```

**Ignore this!** The field exists, just not in introspection.

### Step 2: Type Your Query Manually

Don't rely on autocomplete for aggregate fields. Type them manually:
- `count_id`
- `sum_price`
- `avg_rating`

### Step 3: Run the Query

Click "Run" (or press Ctrl+Enter). The query will execute successfully despite the schema error.

### Step 4: Verify Results

You'll get correct data back, proving the field exists.

## Introspection Query Results

If you run an introspection query:

```graphql
query {
  __type(name: "categories") {
    fields {
      name
    }
  }
}
```

**You'll see:** `id`, `name`, `description`
**You WON'T see:** `count_id`, `avg_id`, `sum_id`, etc.

**BUT** these aggregate fields still work in actual queries!

## Workarounds

### 1. Reference Documentation

Keep `test-aggregates.graphql` and `GRAPHJIN_CORRECT_SYNTAX.md` open as reference. Copy-paste working queries.

### 2. Disable Schema Validation

In your GraphQL client, disable strict schema validation if possible.

### 3. Use GraphiQL Without Validation

Some GraphQL clients allow you to disable validation. The queries will work even if validation fails.

### 4. Generate Custom Schema (Advanced)

You can manually create a schema file that includes aggregate fields:

```graphql
type categories {
  id: ID!
  name: String!
  description: String

  # Manually added aggregate fields (not in GraphJin introspection)
  count_id: Int
  avg_id: Float
  min_id: Int
  max_id: Int
  sum_id: Int
}
```

But this is maintenance-heavy and not recommended.

## What Fields ARE in the Schema?

Run this to see actual schema fields:

```graphql
query {
  __schema {
    types {
      name
      kind
      fields {
        name
      }
    }
  }
}
```

You'll see base table fields but NOT aggregate fields.

## Tested Working Queries

All 19 queries in `test-aggregates.graphql` are confirmed working, even though the schema shows errors. This includes:

- ✅ Simple counts: `{ users { count_id } }`
- ✅ Nested counts: `{ users { posts { count_id } } }`
- ✅ Filtered counts: `{ posts(where: { published: { eq: true } }) { count_id } }`
- ✅ Grouped counts: `{ posts { published count_id } }`
- ✅ Multiple aggregates: `{ posts { count_id avg_id min_id max_id } }`

## Configuration

Ensure introspection is enabled in `config.json`:

```json
{
  "server": {
    "enable_introspection": true
  },
  "graphjin": {
    "enabled": true,
    "disable_functions": false
  }
}
```

This enables GraphJin's introspection, but it still won't show aggregate fields (GraphJin limitation).

## Related Issues

- [GraphJin Discussion #430 - Usage with strict GraphQL clients](https://github.com/dosco/graphjin/discussions/430)
- [GraphQL Introspection - Official Docs](https://graphql.org/learn/introspection/)
- [GraphJin Cheatsheet](https://graphjin.com/posts/cheatsheet)

## Bottom Line

**Ignore schema validation errors for aggregate fields.** If the query is in `test-aggregates.graphql` or follows the `count_<field>` pattern, it works regardless of what the schema says.

Think of aggregate fields as **hidden superpowers** that GraphJin provides but doesn't advertise in its schema. 🦸‍♂️

## Quick Reference Card

Print this and keep it handy:

```
╔══════════════════════════════════════════════╗
║  GraphJin Aggregate Fields (Always Work!)   ║
╠══════════════════════════════════════════════╣
║  count_id        → Count all records         ║
║  count_<field>   → Count non-null values     ║
║  sum_<field>     → Sum numbers               ║
║  avg_<field>     → Average numbers           ║
║  min_<field>     → Minimum value             ║
║  max_<field>     → Maximum value             ║
╠══════════════════════════════════════════════╣
║  NOTE: Schema won't show these fields!       ║
║        Ignore WebUI validation errors!       ║
╚══════════════════════════════════════════════╝
```

## Testing

Run `test-introspection.graphql` to see what's in the schema vs what actually works:

1. Introspection shows: `id`, `name`, `description`
2. But you can query: `count_id`, `avg_id`, `sum_id`, etc.
3. This proves aggregate fields are dynamic/virtual

## Future GraphJin Updates

This is a known limitation. Future versions of GraphJin may:
- Expose aggregate fields in introspection
- Add configuration to control which virtual fields appear in schema
- Provide better strict GraphQL client compatibility

For now, work around it by ignoring schema validation for aggregate queries.
