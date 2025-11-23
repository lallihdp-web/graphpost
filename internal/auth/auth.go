package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/graphpost/graphpost/internal/config"
)

// contextKey is a custom type for context keys
type contextKey string

const (
	// SessionKey is the context key for session data
	SessionKey contextKey = "session"
)

// Session represents an authenticated session
type Session struct {
	UserID          string                 `json:"user_id"`
	Role            string                 `json:"role"`
	AllowedRoles    []string               `json:"allowed_roles"`
	IsAdmin         bool                   `json:"is_admin"`
	Claims          map[string]interface{} `json:"claims"`
	ExpiresAt       time.Time              `json:"expires_at"`
}

// Permission represents a permission rule
type Permission struct {
	Role       string               `json:"role"`
	Table      string               `json:"table"`
	Operation  OperationType        `json:"operation"`
	Columns    []string             `json:"columns,omitempty"`
	Filter     map[string]interface{} `json:"filter,omitempty"`
	Check      map[string]interface{} `json:"check,omitempty"`
	AllowAggregations bool            `json:"allow_aggregations"`
}

// OperationType represents a GraphQL operation type
type OperationType string

const (
	OperationSelect OperationType = "select"
	OperationInsert OperationType = "insert"
	OperationUpdate OperationType = "update"
	OperationDelete OperationType = "delete"
)

// Authenticator handles authentication
type Authenticator struct {
	config      *config.AuthConfig
	permissions map[string]map[string][]*Permission // role -> table -> permissions
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(cfg *config.AuthConfig) *Authenticator {
	return &Authenticator{
		config:      cfg,
		permissions: make(map[string]map[string][]*Permission),
	}
}

// AuthenticateRequest authenticates an HTTP request
func (a *Authenticator) AuthenticateRequest(r *http.Request) (*Session, error) {
	// Check admin secret
	adminSecret := r.Header.Get("X-Admin-Secret")
	if adminSecret != "" && adminSecret == a.config.AdminSecret {
		return &Session{
			Role:    "admin",
			IsAdmin: true,
		}, nil
	}

	// Check JWT token
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		return a.ValidateJWT(token)
	}

	// Check webhook authentication
	if a.config.WebhookURL != "" {
		return a.authenticateViaWebhook(r)
	}

	// Return anonymous session if auth not required
	if !a.config.Enabled {
		return &Session{
			Role: a.config.UnauthorizedRole,
		}, nil
	}

	return nil, errors.New("authentication required")
}

// ValidateJWT validates a JWT token
func (a *Authenticator) ValidateJWT(token string) (*Session, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	// Decode header
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("invalid token header")
	}

	var header map[string]interface{}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, errors.New("invalid token header")
	}

	// Verify algorithm
	alg, ok := header["alg"].(string)
	if !ok || alg != a.config.JWTAlgorithm {
		return nil, errors.New("unsupported algorithm")
	}

	// Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid token payload")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, errors.New("invalid token payload")
	}

	// Verify signature (for HS256)
	if a.config.JWTAlgorithm == "HS256" {
		signature, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			return nil, errors.New("invalid token signature")
		}

		signInput := parts[0] + "." + parts[1]
		h := hmac.New(sha256.New, []byte(a.config.JWTSecret))
		h.Write([]byte(signInput))
		expectedSig := h.Sum(nil)

		if !hmac.Equal(signature, expectedSig) {
			return nil, errors.New("invalid token signature")
		}
	}

	// Check expiration
	if exp, ok := payload["exp"].(float64); ok {
		if time.Unix(int64(exp), 0).Before(time.Now()) {
			return nil, errors.New("token expired")
		}
	}

	// Extract Hasura claims
	claims, ok := payload[a.config.JWTClaimsNamespace].(map[string]interface{})
	if !ok {
		claims = make(map[string]interface{})
	}

	// Build session
	session := &Session{
		Claims: payload,
	}

	// Extract user ID
	if sub, ok := payload["sub"].(string); ok {
		session.UserID = sub
	}

	// Extract role
	if role, ok := claims["x-graphpost-default-role"].(string); ok {
		session.Role = role
	} else {
		session.Role = a.config.DefaultRole
	}

	// Extract allowed roles
	if allowedRoles, ok := claims["x-graphpost-allowed-roles"].([]interface{}); ok {
		for _, r := range allowedRoles {
			if roleStr, ok := r.(string); ok {
				session.AllowedRoles = append(session.AllowedRoles, roleStr)
			}
		}
	}

	// Check if role is allowed
	if !contains(session.AllowedRoles, session.Role) && len(session.AllowedRoles) > 0 {
		session.Role = session.AllowedRoles[0]
	}

	return session, nil
}

// authenticateViaWebhook authenticates via webhook
func (a *Authenticator) authenticateViaWebhook(r *http.Request) (*Session, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", a.config.WebhookURL, nil)
	if err != nil {
		return nil, err
	}

	// Forward relevant headers
	for _, header := range []string{"Authorization", "Cookie", "X-Request-Id"} {
		if val := r.Header.Get(header); val != "" {
			req.Header.Set(header, val)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("webhook authentication failed")
	}

	var webhookResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&webhookResponse); err != nil {
		return nil, fmt.Errorf("invalid webhook response: %w", err)
	}

	// Build session from webhook response
	session := &Session{
		Claims: webhookResponse,
	}

	if role, ok := webhookResponse["X-GraphPost-Role"].(string); ok {
		session.Role = role
	}
	if userID, ok := webhookResponse["X-GraphPost-User-Id"].(string); ok {
		session.UserID = userID
	}

	return session, nil
}

// AddPermission adds a permission rule
func (a *Authenticator) AddPermission(perm *Permission) {
	if a.permissions[perm.Role] == nil {
		a.permissions[perm.Role] = make(map[string][]*Permission)
	}
	a.permissions[perm.Role][perm.Table] = append(a.permissions[perm.Role][perm.Table], perm)
}

// GetPermissions returns permissions for a role and table
func (a *Authenticator) GetPermissions(role, table string, op OperationType) []*Permission {
	if rolePerms, ok := a.permissions[role]; ok {
		if tablePerms, ok := rolePerms[table]; ok {
			var result []*Permission
			for _, p := range tablePerms {
				if p.Operation == op {
					result = append(result, p)
				}
			}
			return result
		}
	}
	return nil
}

// CheckPermission checks if an operation is allowed
func (a *Authenticator) CheckPermission(session *Session, table string, op OperationType) (*Permission, error) {
	if session.IsAdmin {
		return &Permission{
			Role:      "admin",
			Table:     table,
			Operation: op,
		}, nil
	}

	perms := a.GetPermissions(session.Role, table, op)
	if len(perms) == 0 {
		return nil, fmt.Errorf("permission denied: %s on %s for role %s", op, table, session.Role)
	}

	return perms[0], nil
}

// ApplyRowFilter applies row-level security filter to a query
func (a *Authenticator) ApplyRowFilter(session *Session, table string, op OperationType, where map[string]interface{}) map[string]interface{} {
	if session.IsAdmin {
		return where
	}

	perms := a.GetPermissions(session.Role, table, op)
	if len(perms) == 0 || perms[0].Filter == nil {
		return where
	}

	// Merge permission filter with query filter
	permFilter := a.substituteSessionVariables(perms[0].Filter, session)

	if len(where) == 0 {
		return permFilter
	}

	return map[string]interface{}{
		"_and": []interface{}{where, permFilter},
	}
}

// ApplyColumnPermissions filters columns based on permissions
func (a *Authenticator) ApplyColumnPermissions(session *Session, table string, op OperationType, columns []string) []string {
	if session.IsAdmin {
		return columns
	}

	perms := a.GetPermissions(session.Role, table, op)
	if len(perms) == 0 || len(perms[0].Columns) == 0 {
		return columns
	}

	allowedCols := make(map[string]bool)
	for _, col := range perms[0].Columns {
		allowedCols[col] = true
	}

	var filtered []string
	for _, col := range columns {
		if allowedCols[col] {
			filtered = append(filtered, col)
		}
	}

	return filtered
}

// substituteSessionVariables replaces X-Hasura-* variables in filters
func (a *Authenticator) substituteSessionVariables(filter map[string]interface{}, session *Session) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range filter {
		switch v := value.(type) {
		case string:
			if strings.HasPrefix(v, "X-Hasura-") {
				// Substitute session variable
				switch v {
				case "X-Hasura-User-Id":
					result[key] = session.UserID
				case "X-Hasura-Role":
					result[key] = session.Role
				default:
					// Check in claims
					if claims, ok := session.Claims[a.config.JWTClaimsNamespace].(map[string]interface{}); ok {
						if val, ok := claims[strings.ToLower(v)]; ok {
							result[key] = val
						}
					}
				}
			} else {
				result[key] = v
			}
		case map[string]interface{}:
			result[key] = a.substituteSessionVariables(v, session)
		case []interface{}:
			arr := make([]interface{}, len(v))
			for i, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					arr[i] = a.substituteSessionVariables(itemMap, session)
				} else {
					arr[i] = item
				}
			}
			result[key] = arr
		default:
			result[key] = v
		}
	}

	return result
}

// GetSessionFromContext retrieves session from context
func GetSessionFromContext(ctx context.Context) *Session {
	if session, ok := ctx.Value(SessionKey).(*Session); ok {
		return session
	}
	return nil
}

// SetSessionInContext sets session in context
func SetSessionInContext(ctx context.Context, session *Session) context.Context {
	return context.WithValue(ctx, SessionKey, session)
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// PermissionRule represents a permission rule in metadata
type PermissionRule struct {
	Role       string                 `json:"role"`
	Permission PermissionDefinition   `json:"permission"`
}

// PermissionDefinition defines a permission
type PermissionDefinition struct {
	Columns           []string               `json:"columns,omitempty"`
	Filter            map[string]interface{} `json:"filter,omitempty"`
	Check             map[string]interface{} `json:"check,omitempty"`
	Set               map[string]interface{} `json:"set,omitempty"`
	AllowAggregations bool                   `json:"allow_aggregations,omitempty"`
	Limit             *int                   `json:"limit,omitempty"`
}

// TablePermissions holds all permissions for a table
type TablePermissions struct {
	SelectPermissions []PermissionRule `json:"select_permissions,omitempty"`
	InsertPermissions []PermissionRule `json:"insert_permissions,omitempty"`
	UpdatePermissions []PermissionRule `json:"update_permissions,omitempty"`
	DeletePermissions []PermissionRule `json:"delete_permissions,omitempty"`
}
