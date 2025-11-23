# API Reference

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/graphql` | POST | GraphQL query/mutation endpoint |
| `/v1/graphql` | GET | GraphQL query with query params |
| `/v1/graphql/ws` | WebSocket | GraphQL subscriptions |
| `/healthz` | GET | Health check |
| `/v1/metadata` | POST | Schema metadata API |
| `/v1/config` | GET | Configuration info |
| `/console` | GET | Admin console UI |

## GraphQL Endpoint

### POST /v1/graphql

**Request:**
```json
{
  "query": "query { users { id name } }",
  "variables": {},
  "operationName": "MyQuery"
}
```

**Headers:**
| Header | Description | Required |
|--------|-------------|----------|
| `Content-Type` | `application/json` | Yes |
| `X-Admin-Secret` | Admin authentication | Conditional |
| `Authorization` | Bearer JWT token | Conditional |
| `X-GraphPost-Role` | Role override | No |

**Response:**
```json
{
  "data": {
    "users": [
      {"id": 1, "name": "John"},
      {"id": 2, "name": "Jane"}
    ]
  }
}
```

**Error Response:**
```json
{
  "errors": [
    {
      "message": "field \"invalid\" not found",
      "locations": [{"line": 1, "column": 10}],
      "path": ["users"]
    }
  ]
}
```

## Generated GraphQL Schema

For each table `users`, GraphPost generates:

### Query Types

```graphql
type Query {
  # Select multiple rows
  users(
    where: users_bool_exp
    order_by: [users_order_by!]
    limit: Int
    offset: Int
    distinct_on: [users_select_column!]
  ): [users!]!

  # Select by primary key
  users_by_pk(id: Int!): users

  # Aggregate query
  users_aggregate(
    where: users_bool_exp
    order_by: [users_order_by!]
    limit: Int
    offset: Int
  ): users_aggregate!
}
```

### Mutation Types

```graphql
type Mutation {
  # Insert rows
  insert_users(
    objects: [users_insert_input!]!
    on_conflict: users_on_conflict
  ): users_mutation_response

  # Insert single row
  insert_users_one(
    object: users_insert_input!
    on_conflict: users_on_conflict
  ): users

  # Update rows
  update_users(
    where: users_bool_exp!
    _set: users_set_input
    _inc: users_inc_input
  ): users_mutation_response

  # Update by primary key
  update_users_by_pk(
    pk_columns: users_pk_columns_input!
    _set: users_set_input
  ): users

  # Delete rows
  delete_users(where: users_bool_exp!): users_mutation_response

  # Delete by primary key
  delete_users_by_pk(id: Int!): users
}
```

### Subscription Types

```graphql
type Subscription {
  # Subscribe to changes
  users(
    where: users_bool_exp
    order_by: [users_order_by!]
    limit: Int
    offset: Int
  ): [users!]!

  # Subscribe by primary key
  users_by_pk(id: Int!): users

  # Subscribe to aggregate
  users_aggregate(where: users_bool_exp): users_aggregate!
}
```

### Input Types

```graphql
# Boolean expression for filtering
input users_bool_exp {
  _and: [users_bool_exp!]
  _or: [users_bool_exp!]
  _not: users_bool_exp
  id: Int_comparison_exp
  name: String_comparison_exp
  email: String_comparison_exp
  created_at: timestamptz_comparison_exp
}

# Comparison operators
input Int_comparison_exp {
  _eq: Int
  _neq: Int
  _gt: Int
  _gte: Int
  _lt: Int
  _lte: Int
  _in: [Int!]
  _nin: [Int!]
  _is_null: Boolean
}

input String_comparison_exp {
  _eq: String
  _neq: String
  _gt: String
  _gte: String
  _lt: String
  _lte: String
  _in: [String!]
  _nin: [String!]
  _is_null: Boolean
  _like: String
  _ilike: String
  _similar: String
  _regex: String
  _iregex: String
}

# Order by
input users_order_by {
  id: order_by
  name: order_by
  email: order_by
  created_at: order_by
}

enum order_by {
  asc
  asc_nulls_first
  asc_nulls_last
  desc
  desc_nulls_first
  desc_nulls_last
}

# Insert input
input users_insert_input {
  name: String
  email: String
}

# Update input
input users_set_input {
  name: String
  email: String
}

input users_inc_input {
  id: Int  # Increment numeric columns
}

# On conflict
input users_on_conflict {
  constraint: users_constraint!
  update_columns: [users_update_column!]!
  where: users_bool_exp
}
```

## Query Examples

### Basic Query

```graphql
query GetUsers {
  users {
    id
    name
    email
  }
}
```

### Filtered Query

```graphql
query GetActiveUsers {
  users(where: {status: {_eq: "active"}}) {
    id
    name
  }
}
```

### Nested Filters

```graphql
query ComplexFilter {
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

### Pagination

```graphql
query PaginatedUsers {
  users(
    limit: 10,
    offset: 20,
    order_by: {created_at: desc}
  ) {
    id
    name
    created_at
  }
}
```

### Aggregation

```graphql
query UserStats {
  users_aggregate(where: {status: {_eq: "active"}}) {
    aggregate {
      count
      avg {
        age
      }
      max {
        created_at
      }
    }
  }
}
```

### Relationships

```graphql
query UsersWithOrders {
  users {
    id
    name
    orders {
      id
      total
      created_at
    }
  }
}
```

## Mutation Examples

### Insert

```graphql
mutation CreateUser {
  insert_users_one(object: {
    name: "John Doe",
    email: "john@example.com"
  }) {
    id
    name
    email
  }
}
```

### Batch Insert

```graphql
mutation CreateUsers {
  insert_users(objects: [
    {name: "John", email: "john@example.com"},
    {name: "Jane", email: "jane@example.com"}
  ]) {
    affected_rows
    returning {
      id
      name
    }
  }
}
```

### Update

```graphql
mutation UpdateUser {
  update_users(
    where: {id: {_eq: 1}},
    _set: {name: "John Smith"}
  ) {
    affected_rows
    returning {
      id
      name
    }
  }
}
```

### Upsert (Insert or Update)

```graphql
mutation UpsertUser {
  insert_users_one(
    object: {id: 1, name: "John", email: "john@example.com"},
    on_conflict: {
      constraint: users_pkey,
      update_columns: [name, email]
    }
  ) {
    id
    name
  }
}
```

### Delete

```graphql
mutation DeleteUser {
  delete_users(where: {id: {_eq: 1}}) {
    affected_rows
  }
}
```

## Subscription Examples

### Subscribe to Table

```graphql
subscription WatchUsers {
  users(where: {status: {_eq: "active"}}) {
    id
    name
    status
  }
}
```

### WebSocket Connection

```javascript
const ws = new WebSocket('ws://localhost:8080/v1/graphql/ws');

ws.onopen = () => {
  // Initialize connection
  ws.send(JSON.stringify({
    type: 'connection_init',
    payload: {
      headers: {
        'X-Admin-Secret': 'my-secret'
      }
    }
  }));
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);

  if (message.type === 'connection_ack') {
    // Start subscription
    ws.send(JSON.stringify({
      id: '1',
      type: 'start',
      payload: {
        query: `subscription { users { id name } }`
      }
    }));
  }

  if (message.type === 'data') {
    console.log('Data:', message.payload.data);
  }
};
```

## Health Check

### GET /healthz

**Response:**
```json
{
  "status": "ok",
  "database": {
    "status": "healthy",
    "total_connections": 10,
    "idle_connections": 8
  }
}
```

## Metadata API

### POST /v1/metadata

Administrative operations for schema management.

**Track Table:**
```json
{
  "type": "pg_track_table",
  "args": {
    "schema": "public",
    "name": "users"
  }
}
```

**Untrack Table:**
```json
{
  "type": "pg_untrack_table",
  "args": {
    "schema": "public",
    "name": "users"
  }
}
```

**Reload Schema:**
```json
{
  "type": "reload_metadata"
}
```

**Export Metadata:**
```json
{
  "type": "export_metadata"
}
```

## Error Codes

| Code | Description |
|------|-------------|
| `access-denied` | Authentication/authorization failed |
| `validation-failed` | Input validation error |
| `constraint-violation` | Database constraint violated |
| `not-found` | Resource not found |
| `unexpected` | Internal server error |

## Rate Limiting

GraphPost supports rate limiting headers:

```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1609459200
```

## GraphQL Over HTTP Spec

GraphPost follows the [GraphQL over HTTP specification](https://graphql.github.io/graphql-over-http/):

- Accepts `application/json` content type
- Returns `application/json` responses
- Supports GET requests with query string
- Supports POST requests with JSON body
- Returns appropriate HTTP status codes
