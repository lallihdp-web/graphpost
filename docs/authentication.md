# Authentication & Authorization

## Overview

GraphPost supports three authentication methods:
1. **Admin Secret**: Simple header-based authentication
2. **JWT (JSON Web Tokens)**: Stateless token authentication
3. **Webhook**: External authentication service

## Authentication Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        HTTP Request                              │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Check X-Admin-Secret                          │
│                                                                  │
│  Header present? ──► Validate against config.Auth.AdminSecret   │
│  │                                                               │
│  ├── Valid ──► Grant admin access (all permissions)             │
│  └── Invalid ──► Continue to JWT check                          │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Check JWT Token                               │
│                                                                  │
│  Authorization: Bearer <token>                                   │
│  │                                                               │
│  ├── Validate signature (HS256/RS256)                           │
│  ├── Check expiration                                           │
│  └── Extract claims:                                            │
│      ├── x-graphpost-default-role                               │
│      ├── x-graphpost-allowed-roles                              │
│      └── x-graphpost-user-id                                    │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Webhook Auth (Optional)                       │
│                                                                  │
│  POST to config.Auth.WebhookURL with:                           │
│  ├── Original headers                                           │
│  └── Request metadata                                           │
│                                                                  │
│  Response:                                                       │
│  ├── X-GraphPost-Role: <role>                                   │
│  └── X-GraphPost-User-Id: <user_id>                             │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Session Context                               │
│                                                                  │
│  {                                                               │
│    "role": "user",                                              │
│    "user_id": "123",                                            │
│    "allowed_roles": ["user", "moderator"]                       │
│  }                                                               │
└─────────────────────────────────────────────────────────────────┘
```

## Admin Secret Authentication

### Configuration

```go
// internal/config/config.go
type AuthConfig struct {
    AdminSecret string `json:"admin_secret"` // X-Admin-Secret header value
}
```

### Environment Variables

```bash
GRAPHPOST_ADMIN_SECRET="my-super-secret-admin-key"
```

### Usage

```bash
# HTTP request with admin secret
curl -X POST http://localhost:8080/v1/graphql \
  -H "Content-Type: application/json" \
  -H "X-Admin-Secret: my-super-secret-admin-key" \
  -d '{"query": "{ users { id name } }"}'
```

### Implementation

```go
// internal/auth/auth.go

const AdminSecretHeader = "X-Admin-Secret"

func (a *Authenticator) Authenticate(r *http.Request) (*Session, error) {
    // Check admin secret
    adminSecret := r.Header.Get(AdminSecretHeader)
    if adminSecret != "" {
        if adminSecret == a.config.AdminSecret {
            return &Session{
                Role:         "admin",
                IsAdmin:      true,
                AllowedRoles: []string{"admin"},
            }, nil
        }
        return nil, ErrInvalidAdminSecret
    }

    // Continue to other auth methods...
}
```

## JWT Authentication

### JWT Configuration

```go
type AuthConfig struct {
    JWTSecret          string `json:"jwt_secret"`          // For HS256
    JWTPublicKey       string `json:"jwt_public_key"`      // For RS256
    JWTAlgorithm       string `json:"jwt_algorithm"`       // HS256 or RS256
    JWTClaimsNamespace string `json:"jwt_claims_namespace"` // Default: https://graphpost.io/jwt/claims
}
```

### JWT Token Structure

```json
{
  "sub": "user-uuid-123",
  "iat": 1699900000,
  "exp": 1699986400,
  "https://graphpost.io/jwt/claims": {
    "x-graphpost-default-role": "user",
    "x-graphpost-allowed-roles": ["user", "moderator"],
    "x-graphpost-user-id": "123"
  }
}
```

### Supported Claims

| Claim | Description | Required |
|-------|-------------|----------|
| `x-graphpost-default-role` | Default role for the user | Yes |
| `x-graphpost-allowed-roles` | Roles the user can assume | Yes |
| `x-graphpost-user-id` | User identifier | No |
| `x-graphpost-org-id` | Organization identifier | No |

### JWT Validation

```go
// internal/auth/auth.go

func (a *Authenticator) validateJWT(tokenString string) (*Session, error) {
    // Parse token
    token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        // Validate algorithm
        switch a.config.JWTAlgorithm {
        case "HS256":
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
            }
            return []byte(a.config.JWTSecret), nil
        case "RS256":
            if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
            }
            return a.publicKey, nil
        default:
            return nil, fmt.Errorf("unsupported algorithm: %s", a.config.JWTAlgorithm)
        }
    })

    if err != nil {
        return nil, fmt.Errorf("invalid token: %w", err)
    }

    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok || !token.Valid {
        return nil, ErrInvalidToken
    }

    // Extract GraphPost claims
    namespace := a.config.JWTClaimsNamespace
    graphpostClaims, ok := claims[namespace].(map[string]interface{})
    if !ok {
        return nil, ErrMissingClaims
    }

    session := &Session{}

    // Extract default role
    if role, ok := graphpostClaims["x-graphpost-default-role"].(string); ok {
        session.Role = role
    }

    // Extract allowed roles
    if roles, ok := graphpostClaims["x-graphpost-allowed-roles"].([]interface{}); ok {
        for _, r := range roles {
            session.AllowedRoles = append(session.AllowedRoles, r.(string))
        }
    }

    // Extract user ID
    if userID, ok := graphpostClaims["x-graphpost-user-id"].(string); ok {
        session.UserID = userID
    }

    return session, nil
}
```

### Usage

```bash
# Get JWT from your auth server
TOKEN="eyJhbGciOiJIUzI1NiIs..."

# Request with JWT
curl -X POST http://localhost:8080/v1/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"query": "{ users { id name } }"}'
```

### Role Override

Users can override their role using the `X-GraphPost-Role` header, but only to a role in their `allowed_roles`:

```bash
curl -X POST http://localhost:8080/v1/graphql \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-GraphPost-Role: moderator" \
  -d '{"query": "..."}'
```

## Webhook Authentication

### Configuration

```go
type AuthConfig struct {
    WebhookURL string `json:"webhook_url"` // External auth service URL
}
```

### Webhook Request

GraphPost sends a POST request to your webhook URL:

```json
{
  "headers": {
    "Authorization": "Bearer user-token",
    "X-Custom-Header": "value"
  },
  "request": {
    "method": "POST",
    "path": "/v1/graphql"
  }
}
```

### Expected Response

Your webhook should return:

```json
{
  "X-GraphPost-Role": "user",
  "X-GraphPost-User-Id": "123"
}
```

Or an error:

```json
{
  "error": "Unauthorized"
}
```

### Implementation

```go
// internal/auth/auth.go

func (a *Authenticator) webhookAuth(r *http.Request) (*Session, error) {
    // Build webhook payload
    payload := map[string]interface{}{
        "headers": extractHeaders(r),
        "request": map[string]string{
            "method": r.Method,
            "path":   r.URL.Path,
        },
    }

    // Call webhook
    resp, err := a.httpClient.Post(
        a.config.WebhookURL,
        "application/json",
        bytes.NewReader(jsonEncode(payload)),
    )
    if err != nil {
        return nil, fmt.Errorf("webhook call failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, ErrWebhookDenied
    }

    // Parse response
    var result map[string]string
    json.NewDecoder(resp.Body).Decode(&result)

    return &Session{
        Role:   result["X-GraphPost-Role"],
        UserID: result["X-GraphPost-User-Id"],
    }, nil
}
```

## Authorization (Permissions)

### Permission Rules

```go
// internal/auth/auth.go

type PermissionRule struct {
    Role        string                 `json:"role"`
    Permission  string                 `json:"permission"` // select, insert, update, delete
    Columns     []string               `json:"columns"`    // Allowed columns
    Filter      map[string]interface{} `json:"filter"`     // Row-level filter
    Check       map[string]interface{} `json:"check"`      // Validation for mutations
    Set         map[string]interface{} `json:"set"`        // Column presets
}
```

### Example Permission Configuration

```json
{
  "tables": {
    "users": {
      "select_permissions": [
        {
          "role": "user",
          "permission": "select",
          "columns": ["id", "name", "email"],
          "filter": {
            "id": {"_eq": "X-GraphPost-User-Id"}
          }
        }
      ],
      "insert_permissions": [
        {
          "role": "user",
          "permission": "insert",
          "columns": ["name", "email"],
          "set": {
            "created_by": "X-GraphPost-User-Id"
          }
        }
      ],
      "update_permissions": [
        {
          "role": "user",
          "permission": "update",
          "columns": ["name", "email"],
          "filter": {
            "id": {"_eq": "X-GraphPost-User-Id"}
          }
        }
      ],
      "delete_permissions": [
        {
          "role": "admin",
          "permission": "delete",
          "filter": {}
        }
      ]
    }
  }
}
```

### Permission Enforcement

```go
// Apply permission filter to query
func (r *Resolver) applyPermissions(params *QueryParams, session *Session) error {
    rule := r.getPermissionRule(params.TableName, "select", session.Role)
    if rule == nil {
        return ErrPermissionDenied
    }

    // Filter allowed columns
    if len(rule.Columns) > 0 {
        params.Columns = intersect(params.Columns, rule.Columns)
    }

    // Apply row-level filter
    if len(rule.Filter) > 0 {
        // Replace session variables
        filter := substituteSessionVars(rule.Filter, session)
        params.Where = mergeFilters(params.Where, filter)
    }

    return nil
}
```

## Session Variables

Session variables are available in permission rules and can be substituted:

| Variable | Description |
|----------|-------------|
| `X-GraphPost-User-Id` | Current user's ID |
| `X-GraphPost-Role` | Current user's role |
| Custom claims | Any claim from JWT |

### Example Usage in Filters

```json
{
  "filter": {
    "user_id": {"_eq": "X-GraphPost-User-Id"},
    "organization_id": {"_eq": "X-GraphPost-Org-Id"}
  }
}
```

## Unauthenticated Access

For public endpoints, you can configure anonymous access:

```go
type AuthConfig struct {
    AnonymousRole string `json:"anonymous_role"` // Role for unauthenticated users
}
```

Requests without credentials will be assigned the anonymous role with its permissions.

## Security Best Practices

1. **Always use HTTPS** in production
2. **Rotate secrets** regularly
3. **Use short JWT expiration** times
4. **Validate all inputs** server-side
5. **Log authentication failures** for monitoring
6. **Use webhook auth** for complex scenarios
7. **Implement rate limiting** for auth endpoints
