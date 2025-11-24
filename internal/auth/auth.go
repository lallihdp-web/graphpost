package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"net/http"
	"strings"
	"sync"
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

	// JWT key management
	rsaPublicKey   *rsa.PublicKey
	ecdsaPublicKey *ecdsa.PublicKey
	hmacSecret     []byte

	// JWKS cache
	jwksMu       sync.RWMutex
	jwksCache    *JWKSet
	jwksCacheExp time.Time
}

// JWKSet represents a JSON Web Key Set
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key
type JWK struct {
	Kty string `json:"kty"`          // Key type: RSA, EC
	Use string `json:"use"`          // Key use: sig
	Kid string `json:"kid"`          // Key ID
	Alg string `json:"alg"`          // Algorithm
	N   string `json:"n,omitempty"`  // RSA modulus
	E   string `json:"e,omitempty"`  // RSA exponent
	X   string `json:"x,omitempty"`  // EC X coordinate
	Y   string `json:"y,omitempty"`  // EC Y coordinate
	Crv string `json:"crv,omitempty"` // EC curve
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(cfg *config.AuthConfig) *Authenticator {
	a := &Authenticator{
		config:      cfg,
		permissions: make(map[string]map[string][]*Permission),
	}

	// Initialize keys based on configuration
	a.initializeKeys()

	return a
}

// initializeKeys parses and stores cryptographic keys
func (a *Authenticator) initializeKeys() {
	// Check new JWT config first
	if a.config.JWT != nil && a.config.JWT.Key != "" {
		a.parseKey(a.config.JWT.Key, a.config.JWT.Type)
		return
	}

	// Fall back to legacy config
	if a.config.JWTSecret != "" {
		algo := a.config.JWTAlgorithm
		if algo == "" {
			algo = "HS256"
		}
		a.parseKey(a.config.JWTSecret, algo)
	}
}

// parseKey parses a key based on the algorithm type
func (a *Authenticator) parseKey(key string, algo string) {
	switch {
	case strings.HasPrefix(algo, "HS"):
		a.hmacSecret = []byte(key)

	case strings.HasPrefix(algo, "RS"):
		// Try to parse as PEM
		block, _ := pem.Decode([]byte(key))
		if block != nil {
			if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
				if rsaPub, ok := pub.(*rsa.PublicKey); ok {
					a.rsaPublicKey = rsaPub
				}
			}
			// Also try parsing as PKCS1
			if a.rsaPublicKey == nil {
				if rsaPub, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
					a.rsaPublicKey = rsaPub
				}
			}
		}

	case strings.HasPrefix(algo, "ES"):
		block, _ := pem.Decode([]byte(key))
		if block != nil {
			if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
				if ecPub, ok := pub.(*ecdsa.PublicKey); ok {
					a.ecdsaPublicKey = ecPub
				}
			}
		}
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

	// Extract JWT token based on configuration
	token := a.extractToken(r)
	if token != "" {
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

// extractToken extracts JWT token from the request based on configuration
func (a *Authenticator) extractToken(r *http.Request) string {
	// Check new JWT config for custom header
	if a.config.JWT != nil && a.config.JWT.Header != nil {
		header := a.config.JWT.Header
		switch header.Type {
		case "Cookie":
			if cookie, err := r.Cookie(header.Name); err == nil {
				return cookie.Value
			}
		default: // "Authorization" or custom header
			headerName := header.Name
			if headerName == "" {
				headerName = "Authorization"
			}
			value := r.Header.Get(headerName)
			if strings.HasPrefix(value, "Bearer ") {
				return strings.TrimPrefix(value, "Bearer ")
			}
			return value
		}
	}

	// Default: check Authorization header
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	return ""
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

	// Get algorithm from token
	alg, ok := header["alg"].(string)
	if !ok {
		return nil, errors.New("missing algorithm in token")
	}

	// Get expected algorithm from config
	expectedAlg := a.getExpectedAlgorithm()
	if expectedAlg != "" && alg != expectedAlg {
		return nil, fmt.Errorf("algorithm mismatch: expected %s, got %s", expectedAlg, alg)
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

	// Check if verification should be skipped (DEVELOPMENT ONLY)
	skipVerification := a.config.JWT != nil && a.config.JWT.SkipVerification
	skipExpirationCheck := a.config.JWT != nil && a.config.JWT.SkipExpirationCheck

	// Get key ID from header for JWKS lookup
	kid, _ := header["kid"].(string)

	// Verify signature (unless skipped for testing)
	if !skipVerification {
		signInput := parts[0] + "." + parts[1]
		signature, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			return nil, errors.New("invalid token signature")
		}

		if err := a.verifySignature(alg, signInput, signature, kid); err != nil {
			return nil, fmt.Errorf("signature verification failed: %w", err)
		}
	}

	// Get allowed clock skew
	allowedSkew := 0
	if a.config.JWT != nil {
		allowedSkew = a.config.JWT.AllowedSkew
	}

	// Check expiration (unless skipped for testing)
	if !skipExpirationCheck {
		if exp, ok := payload["exp"].(float64); ok {
			expTime := time.Unix(int64(exp), 0).Add(time.Duration(allowedSkew) * time.Second)
			if expTime.Before(time.Now()) {
				return nil, errors.New("token expired")
			}
		}

		// Check not before
		if nbf, ok := payload["nbf"].(float64); ok {
			nbfTime := time.Unix(int64(nbf), 0).Add(-time.Duration(allowedSkew) * time.Second)
			if nbfTime.After(time.Now()) {
				return nil, errors.New("token not yet valid")
			}
		}
	}

	// Validate issuer if configured
	if a.config.JWT != nil && a.config.JWT.Issuer != "" {
		iss, _ := payload["iss"].(string)
		if iss != a.config.JWT.Issuer {
			return nil, fmt.Errorf("invalid issuer: expected %s, got %s", a.config.JWT.Issuer, iss)
		}
	}

	// Validate audience if configured
	if a.config.JWT != nil && a.config.JWT.Audience != "" {
		if !a.validateAudience(payload, a.config.JWT.Audience) {
			return nil, errors.New("invalid audience")
		}
	}

	// Build session from claims
	return a.buildSessionFromPayload(payload)
}

// getExpectedAlgorithm returns the expected JWT algorithm
func (a *Authenticator) getExpectedAlgorithm() string {
	if a.config.JWT != nil && a.config.JWT.Type != "" {
		return a.config.JWT.Type
	}
	return a.config.JWTAlgorithm
}

// validateAudience checks if the token audience matches the expected value
func (a *Authenticator) validateAudience(payload map[string]interface{}, expected string) bool {
	aud := payload["aud"]
	switch v := aud.(type) {
	case string:
		return v == expected
	case []interface{}:
		for _, a := range v {
			if str, ok := a.(string); ok && str == expected {
				return true
			}
		}
	}
	return false
}

// verifySignature verifies the JWT signature based on the algorithm
func (a *Authenticator) verifySignature(alg, signInput string, signature []byte, kid string) error {
	switch alg {
	case "HS256":
		return a.verifyHMAC(signInput, signature, sha256.New, 256)
	case "HS384":
		return a.verifyHMAC(signInput, signature, sha512.New384, 384)
	case "HS512":
		return a.verifyHMAC(signInput, signature, sha512.New, 512)
	case "RS256":
		return a.verifyRSA(signInput, signature, crypto.SHA256, kid)
	case "RS384":
		return a.verifyRSA(signInput, signature, crypto.SHA384, kid)
	case "RS512":
		return a.verifyRSA(signInput, signature, crypto.SHA512, kid)
	case "ES256":
		return a.verifyECDSA(signInput, signature, crypto.SHA256, kid)
	case "ES384":
		return a.verifyECDSA(signInput, signature, crypto.SHA384, kid)
	case "ES512":
		return a.verifyECDSA(signInput, signature, crypto.SHA512, kid)
	default:
		return fmt.Errorf("unsupported algorithm: %s", alg)
	}
}

// verifyHMAC verifies an HMAC signature
func (a *Authenticator) verifyHMAC(signInput string, signature []byte, hashFunc func() hash.Hash, _ int) error {
	if len(a.hmacSecret) == 0 {
		return errors.New("HMAC secret not configured")
	}

	h := hmac.New(hashFunc, a.hmacSecret)
	h.Write([]byte(signInput))
	expectedSig := h.Sum(nil)

	if !hmac.Equal(signature, expectedSig) {
		return errors.New("invalid signature")
	}
	return nil
}

// verifyRSA verifies an RSA signature
func (a *Authenticator) verifyRSA(signInput string, signature []byte, hashType crypto.Hash, kid string) error {
	pubKey := a.rsaPublicKey

	// Try to get key from JWKS if kid is provided
	if kid != "" && a.config.JWT != nil && a.config.JWT.JWKUrl != "" {
		if jwkKey, err := a.getJWKSKey(kid); err == nil {
			if rsaKey, ok := jwkKey.(*rsa.PublicKey); ok {
				pubKey = rsaKey
			}
		}
	}

	if pubKey == nil {
		return errors.New("RSA public key not configured")
	}

	h := hashType.New()
	h.Write([]byte(signInput))
	hashed := h.Sum(nil)

	return rsa.VerifyPKCS1v15(pubKey, hashType, hashed, signature)
}

// verifyECDSA verifies an ECDSA signature
func (a *Authenticator) verifyECDSA(signInput string, signature []byte, hashType crypto.Hash, kid string) error {
	pubKey := a.ecdsaPublicKey

	// Try to get key from JWKS if kid is provided
	if kid != "" && a.config.JWT != nil && a.config.JWT.JWKUrl != "" {
		if jwkKey, err := a.getJWKSKey(kid); err == nil {
			if ecKey, ok := jwkKey.(*ecdsa.PublicKey); ok {
				pubKey = ecKey
			}
		}
	}

	if pubKey == nil {
		return errors.New("ECDSA public key not configured")
	}

	h := hashType.New()
	h.Write([]byte(signInput))
	hashed := h.Sum(nil)

	// ECDSA signatures are (r, s) concatenated
	keySize := pubKey.Params().BitSize / 8
	if keySize%8 != 0 {
		keySize++
	}

	if len(signature) != 2*keySize {
		return errors.New("invalid signature length")
	}

	r := new(big.Int).SetBytes(signature[:keySize])
	s := new(big.Int).SetBytes(signature[keySize:])

	if !ecdsa.Verify(pubKey, hashed, r, s) {
		return errors.New("invalid signature")
	}
	return nil
}

// getJWKSKey fetches and caches JWKS, then returns the key with the given kid
func (a *Authenticator) getJWKSKey(kid string) (interface{}, error) {
	a.jwksMu.RLock()
	if a.jwksCache != nil && time.Now().Before(a.jwksCacheExp) {
		defer a.jwksMu.RUnlock()
		return a.findKeyInJWKS(kid)
	}
	a.jwksMu.RUnlock()

	// Fetch JWKS
	a.jwksMu.Lock()
	defer a.jwksMu.Unlock()

	// Double check after acquiring write lock
	if a.jwksCache != nil && time.Now().Before(a.jwksCacheExp) {
		return a.findKeyInJWKS(kid)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(a.config.JWT.JWKUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	var jwks JWKSet
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to decode JWKS: %w", err)
	}

	a.jwksCache = &jwks
	a.jwksCacheExp = time.Now().Add(5 * time.Minute) // Cache for 5 minutes

	return a.findKeyInJWKS(kid)
}

// findKeyInJWKS finds a key by kid in the cached JWKS
func (a *Authenticator) findKeyInJWKS(kid string) (interface{}, error) {
	if a.jwksCache == nil {
		return nil, errors.New("JWKS not loaded")
	}

	for _, key := range a.jwksCache.Keys {
		if key.Kid == kid || kid == "" {
			return a.jwkToPublicKey(key)
		}
	}
	return nil, fmt.Errorf("key with kid %s not found", kid)
}

// jwkToPublicKey converts a JWK to a public key
func (a *Authenticator) jwkToPublicKey(jwk JWK) (interface{}, error) {
	switch jwk.Kty {
	case "RSA":
		// Decode N and E
		nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
		if err != nil {
			return nil, fmt.Errorf("invalid RSA modulus: %w", err)
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
		if err != nil {
			return nil, fmt.Errorf("invalid RSA exponent: %w", err)
		}

		n := new(big.Int).SetBytes(nBytes)
		e := 0
		for _, b := range eBytes {
			e = e*256 + int(b)
		}

		return &rsa.PublicKey{N: n, E: e}, nil

	case "EC":
		xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
		if err != nil {
			return nil, fmt.Errorf("invalid EC X: %w", err)
		}
		yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
		if err != nil {
			return nil, fmt.Errorf("invalid EC Y: %w", err)
		}

		x := new(big.Int).SetBytes(xBytes)
		y := new(big.Int).SetBytes(yBytes)

		// Get curve based on crv
		var curve interface{ Params() *struct{ BitSize int } }
		switch jwk.Crv {
		case "P-256":
			return &ecdsa.PublicKey{X: x, Y: y}, nil
		case "P-384":
			return &ecdsa.PublicKey{X: x, Y: y}, nil
		case "P-521":
			return &ecdsa.PublicKey{X: x, Y: y}, nil
		default:
			return nil, fmt.Errorf("unsupported curve: %s", jwk.Crv)
		}
		_ = curve // Unused, handled above

	default:
		return nil, fmt.Errorf("unsupported key type: %s", jwk.Kty)
	}
}

// buildSessionFromPayload builds a session from JWT payload
func (a *Authenticator) buildSessionFromPayload(payload map[string]interface{}) (*Session, error) {
	session := &Session{
		Claims: payload,
	}

	// Get claims namespace
	claimsNamespace := a.getClaimsNamespace()

	// Try claims mapping first (new config)
	if a.config.JWT != nil && len(a.config.JWT.ClaimsMap) > 0 {
		a.applyClaimsMap(session, payload)
	}

	// Extract claims from namespace
	var claims map[string]interface{}
	if claimsNamespace != "" {
		claims, _ = payload[claimsNamespace].(map[string]interface{})
	}
	if claims == nil {
		claims = make(map[string]interface{})
	}

	// Extract user ID (from 'sub' claim or mapped)
	if session.UserID == "" {
		if sub, ok := payload["sub"].(string); ok {
			session.UserID = sub
		}
	}

	// Extract role from claims (check multiple keys for compatibility)
	if session.Role == "" {
		session.Role = a.extractRoleFromClaims(claims)
	}

	// Extract allowed roles
	if len(session.AllowedRoles) == 0 {
		session.AllowedRoles = a.extractAllowedRoles(claims)
	}

	// Fallback to default role
	if session.Role == "" {
		session.Role = a.config.DefaultRole
	}

	// Validate role is in allowed roles
	if len(session.AllowedRoles) > 0 && !contains(session.AllowedRoles, session.Role) {
		session.Role = session.AllowedRoles[0]
	}

	// Check expiration for session
	if exp, ok := payload["exp"].(float64); ok {
		session.ExpiresAt = time.Unix(int64(exp), 0)
	}

	return session, nil
}

// getClaimsNamespace returns the claims namespace from config
func (a *Authenticator) getClaimsNamespace() string {
	if a.config.JWT != nil && a.config.JWT.ClaimsNamespace != "" {
		return a.config.JWT.ClaimsNamespace
	}
	return a.config.JWTClaimsNamespace
}

// applyClaimsMap applies custom claims mapping to session
func (a *Authenticator) applyClaimsMap(session *Session, payload map[string]interface{}) {
	for claimName, mapping := range a.config.JWT.ClaimsMap {
		value := a.extractValueByPath(payload, mapping.Path)
		if value == nil {
			value = mapping.Default
		}

		switch claimName {
		case "x-hasura-user-id", "x-graphpost-user-id":
			if str, ok := value.(string); ok {
				session.UserID = str
			}
		case "x-hasura-default-role", "x-graphpost-default-role":
			if str, ok := value.(string); ok {
				session.Role = str
			}
		case "x-hasura-allowed-roles", "x-graphpost-allowed-roles":
			if roles, ok := value.([]interface{}); ok {
				for _, r := range roles {
					if str, ok := r.(string); ok {
						session.AllowedRoles = append(session.AllowedRoles, str)
					}
				}
			}
		}
	}
}

// extractValueByPath extracts a value from payload using a simple JSON path
// Supports paths like "$.user.id" or "user.id"
func (a *Authenticator) extractValueByPath(payload map[string]interface{}, path string) interface{} {
	if path == "" {
		return nil
	}

	// Remove $. prefix if present
	path = strings.TrimPrefix(path, "$.")

	parts := strings.Split(path, ".")
	var current interface{} = payload

	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return nil
		}
	}

	return current
}

// extractRoleFromClaims extracts role from claims with multiple key support
func (a *Authenticator) extractRoleFromClaims(claims map[string]interface{}) string {
	// Check multiple possible role keys for Hasura compatibility
	roleKeys := []string{
		"x-graphpost-default-role",
		"x-hasura-default-role",
		"role",
		"default-role",
	}

	for _, key := range roleKeys {
		if role, ok := claims[key].(string); ok && role != "" {
			return role
		}
	}
	return ""
}

// extractAllowedRoles extracts allowed roles from claims
func (a *Authenticator) extractAllowedRoles(claims map[string]interface{}) []string {
	// Check multiple possible keys
	roleKeys := []string{
		"x-graphpost-allowed-roles",
		"x-hasura-allowed-roles",
		"allowed-roles",
		"roles",
	}

	for _, key := range roleKeys {
		if allowedRoles, ok := claims[key].([]interface{}); ok {
			var roles []string
			for _, r := range allowedRoles {
				if roleStr, ok := r.(string); ok {
					roles = append(roles, roleStr)
				}
			}
			if len(roles) > 0 {
				return roles
			}
		}
	}
	return nil
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
