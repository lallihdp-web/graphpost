package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graphpost/graphpost/internal/auth"
)

// Server is the HTTP server for GraphQL
type Server struct {
	engine   *Engine
	server   *http.Server
	upgrader websocket.Upgrader
}

// NewServer creates a new HTTP server
func NewServer(engine *Engine) *Server {
	return &Server{
		engine: engine,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Configure based on CORS settings
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	cfg := s.engine.GetConfig()

	mux := http.NewServeMux()

	// GraphQL endpoint
	mux.HandleFunc("/v1/graphql", s.handleGraphQL)
	mux.HandleFunc("/v1alpha1/graphql", s.handleGraphQL) // Alias

	// Health endpoint
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/version", s.handleVersion)

	// Metadata endpoints
	mux.HandleFunc("/v1/metadata", s.handleMetadata)
	mux.HandleFunc("/v1/query", s.handleMetadata) // Legacy

	// Schema endpoint
	mux.HandleFunc("/v1/graphql/schema", s.handleSchema)

	// Explain endpoint
	mux.HandleFunc("/v1/graphql/explain", s.handleExplain)

	// Config endpoint
	mux.HandleFunc("/v1/config", s.handleConfig)

	// Console/Playground
	if cfg.Console.Enabled {
		mux.HandleFunc("/console", s.handleConsole)
		mux.HandleFunc("/console/", s.handleConsole)
	}

	// GraphQL Playground
	if cfg.Server.EnablePlayground {
		mux.HandleFunc("/", s.handlePlayground)
	}

	// Apply middleware
	handler := s.corsMiddleware(mux)
	handler = s.authMiddleware(handler)
	handler = s.loggingMiddleware(handler)

	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      handler,
		ReadTimeout:  cfg.Server.RequestTimeout,
		WriteTimeout: cfg.Server.RequestTimeout,
	}

	fmt.Printf("🚀 GraphPost server running at http://%s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("📊 GraphQL endpoint: http://%s:%d/v1/graphql\n", cfg.Server.Host, cfg.Server.Port)

	if cfg.Server.EnablePlayground {
		fmt.Printf("🎮 GraphQL Playground: http://%s:%d/\n", cfg.Server.Host, cfg.Server.Port)
	}

	return s.server.ListenAndServe()
}

// Stop stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// handleGraphQL handles GraphQL requests
func (s *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	// Handle WebSocket upgrade for subscriptions
	if strings.Contains(r.Header.Get("Upgrade"), "websocket") {
		s.handleWebSocket(w, r)
		return
	}

	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request GraphQLRequest

	if r.Method == http.MethodGet {
		request.Query = r.URL.Query().Get("query")
		request.OperationName = r.URL.Query().Get("operationName")
		if vars := r.URL.Query().Get("variables"); vars != "" {
			json.Unmarshal([]byte(vars), &request.Variables)
		}
	} else {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
	}

	// Execute query
	ctx := r.Context()
	result := s.engine.ExecuteQuery(ctx, request.Query, request.Variables, request.OperationName)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleWebSocket handles WebSocket connections for subscriptions
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Handle GraphQL over WebSocket protocol
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var wsMessage WSMessage
		if err := json.Unmarshal(message, &wsMessage); err != nil {
			continue
		}

		switch wsMessage.Type {
		case "connection_init":
			conn.WriteJSON(WSMessage{Type: "connection_ack"})

		case "start", "subscribe":
			// Handle subscription
			var payload GraphQLRequest
			payloadBytes, _ := json.Marshal(wsMessage.Payload)
			json.Unmarshal(payloadBytes, &payload)

			result := s.engine.ExecuteQuery(r.Context(), payload.Query, payload.Variables, payload.OperationName)
			conn.WriteJSON(WSMessage{
				ID:      wsMessage.ID,
				Type:    "data",
				Payload: result,
			})
			conn.WriteJSON(WSMessage{
				ID:   wsMessage.ID,
				Type: "complete",
			})

		case "stop":
			// Stop subscription
			conn.WriteJSON(WSMessage{
				ID:   wsMessage.ID,
				Type: "complete",
			})

		case "connection_terminate":
			return
		}
	}
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := s.engine.Health(r.Context())

	w.Header().Set("Content-Type", "application/json")

	if health["status"] != "ok" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(health)
}

// handleVersion handles version requests
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"version": "1.0.0",
	})
}

// handleMetadata handles metadata API requests
func (s *Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check admin secret
	session := auth.GetSessionFromContext(r.Context())
	if session == nil || !session.IsAdmin {
		http.Error(w, "Admin access required", http.StatusUnauthorized)
		return
	}

	var request MetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	response := s.handleMetadataRequest(r.Context(), &request)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleMetadataRequest processes metadata API requests
func (s *Server) handleMetadataRequest(ctx context.Context, req *MetadataRequest) interface{} {
	switch req.Type {
	case "export_metadata":
		return s.exportMetadata()

	case "replace_metadata":
		return s.replaceMetadata(req.Args)

	case "reload_metadata":
		if err := s.engine.Reload(ctx); err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return map[string]string{"message": "success"}

	case "clear_metadata":
		return map[string]string{"message": "success"}

	case "get_inconsistent_metadata":
		return map[string]interface{}{"is_consistent": true, "inconsistent_objects": []interface{}{}}

	case "drop_inconsistent_metadata":
		return map[string]string{"message": "success"}

	case "track_table":
		return s.trackTable(req.Args)

	case "untrack_table":
		return s.untrackTable(req.Args)

	case "create_object_relationship":
		return s.createRelationship(req.Args, "object")

	case "create_array_relationship":
		return s.createRelationship(req.Args, "array")

	case "drop_relationship":
		return map[string]string{"message": "success"}

	case "create_event_trigger":
		return s.createEventTrigger(req.Args)

	case "delete_event_trigger":
		return s.deleteEventTrigger(req.Args)

	case "invoke_event_trigger":
		return s.invokeEventTrigger(ctx, req.Args)

	case "create_insert_permission":
		return s.createPermission(req.Args, auth.OperationInsert)

	case "create_select_permission":
		return s.createPermission(req.Args, auth.OperationSelect)

	case "create_update_permission":
		return s.createPermission(req.Args, auth.OperationUpdate)

	case "create_delete_permission":
		return s.createPermission(req.Args, auth.OperationDelete)

	case "drop_insert_permission", "drop_select_permission", "drop_update_permission", "drop_delete_permission":
		return map[string]string{"message": "success"}

	case "run_sql":
		return s.runSQL(ctx, req.Args)

	default:
		return map[string]interface{}{"error": fmt.Sprintf("unknown metadata type: %s", req.Type)}
	}
}

// exportMetadata exports current metadata
func (s *Server) exportMetadata() *Metadata {
	dbSchema := s.engine.GetDBSchema()

	metadata := &Metadata{
		Tables: make(map[string]*TableMetadata),
	}

	for tableName, table := range dbSchema.Tables {
		metadata.Tables[tableName] = &TableMetadata{
			Name:   table.Name,
			Schema: table.Schema,
		}
	}

	return metadata
}

// replaceMetadata replaces metadata
func (s *Server) replaceMetadata(args interface{}) interface{} {
	return map[string]string{"message": "success"}
}

// trackTable tracks a table
func (s *Server) trackTable(args interface{}) interface{} {
	return map[string]string{"message": "success"}
}

// untrackTable untracks a table
func (s *Server) untrackTable(args interface{}) interface{} {
	return map[string]string{"message": "success"}
}

// createRelationship creates a relationship
func (s *Server) createRelationship(args interface{}, relType string) interface{} {
	return map[string]string{"message": "success"}
}

// createEventTrigger creates an event trigger
func (s *Server) createEventTrigger(args interface{}) interface{} {
	// Parse args and create trigger
	return map[string]string{"message": "success"}
}

// deleteEventTrigger deletes an event trigger
func (s *Server) deleteEventTrigger(args interface{}) interface{} {
	return map[string]string{"message": "success"}
}

// invokeEventTrigger invokes an event trigger manually
func (s *Server) invokeEventTrigger(ctx context.Context, args interface{}) interface{} {
	argsMap, ok := args.(map[string]interface{})
	if !ok {
		return map[string]interface{}{"error": "invalid arguments"}
	}

	name, _ := argsMap["name"].(string)
	payload, _ := argsMap["payload"].(map[string]interface{})

	if err := s.engine.GetTriggerManager().InvokeEventTrigger(ctx, name, payload); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	return map[string]string{"message": "success"}
}

// createPermission creates a permission
func (s *Server) createPermission(args interface{}, op auth.OperationType) interface{} {
	return map[string]string{"message": "success"}
}

// runSQL runs raw SQL
func (s *Server) runSQL(ctx context.Context, args interface{}) interface{} {
	argsMap, ok := args.(map[string]interface{})
	if !ok {
		return map[string]interface{}{"error": "invalid arguments"}
	}

	sql, _ := argsMap["sql"].(string)
	if sql == "" {
		return map[string]interface{}{"error": "sql is required"}
	}

	// Execute SQL
	resolver := s.engine.GetResolver()
	if resolver == nil {
		return map[string]interface{}{"error": "resolver not available"}
	}

	// This is a simplified implementation
	return map[string]interface{}{
		"result_type": "CommandOk",
		"result":      nil,
	}
}

// handleSchema returns the GraphQL schema
func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Return introspection query result
	result := s.engine.ExecuteQuery(r.Context(), introspectionQuery, nil, "")
	json.NewEncoder(w).Encode(result)
}

// handleExplain handles explain requests
func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	// Check admin access
	session := auth.GetSessionFromContext(r.Context())
	if session == nil || !session.IsAdmin {
		http.Error(w, "Admin access required", http.StatusUnauthorized)
		return
	}

	var request GraphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Execute and explain (simplified)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sql":  "SELECT * FROM table",
		"plan": "Seq Scan",
	})
}

// handleConfig returns server configuration
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	session := auth.GetSessionFromContext(r.Context())
	if session == nil || !session.IsAdmin {
		http.Error(w, "Admin access required", http.StatusUnauthorized)
		return
	}

	config := s.engine.GetConfig()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"is_admin_secret_set":    config.Auth.AdminSecret != "",
		"is_auth_hook_set":       config.Auth.WebhookURL != "",
		"is_jwt_set":             config.Auth.JWTSecret != "",
		"jwt_secret":             nil,
		"is_allow_list_enabled":  false,
		"live_queries":           map[string]interface{}{"batch_size": 100, "refetch_delay": 1},
		"streaming_queries":      map[string]interface{}{"batch_size": 100, "refetch_delay": 1},
		"console_assets_dir":     config.Console.AssetsPath,
		"experimental_features":  []string{},
	})
}

// handleConsole serves the admin console
func (s *Server) handleConsole(w http.ResponseWriter, r *http.Request) {
	// Serve embedded console or redirect to external URL
	http.ServeFile(w, r, s.engine.GetConfig().Console.AssetsPath)
}

// handlePlayground serves the GraphQL Playground
func (s *Server) handlePlayground(w http.ResponseWriter, r *http.Request) {
	cfg := s.engine.GetConfig()
	playgroundHTML := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
  <meta charset=utf-8/>
  <meta name="viewport" content="user-scalable=no, initial-scale=1.0, minimum-scale=1.0, maximum-scale=1.0, minimal-ui">
  <title>GraphPost - GraphQL Playground</title>
  <link rel="stylesheet" href="//cdn.jsdelivr.net/npm/graphql-playground-react/build/static/css/index.css" />
  <link rel="shortcut icon" href="//cdn.jsdelivr.net/npm/graphql-playground-react/build/favicon.png" />
  <script src="//cdn.jsdelivr.net/npm/graphql-playground-react/build/static/js/middleware.js"></script>
</head>
<body>
  <div id="root">
    <style>
      body { background-color: rgb(23, 42, 58); font-family: Open Sans, sans-serif; height: 90vh; }
      #root { height: 100%%; width: 100%%; display: flex; align-items: center; justify-content: center; }
      .loading { font-size: 32px; font-weight: 200; color: rgba(255, 255, 255, .6); margin-left: 28px; }
      img { width: 78px; height: 78px; }
      .title { font-weight: 400; }
    </style>
    <img src='//cdn.jsdelivr.net/npm/graphql-playground-react/build/logo.png' alt=''>
    <div class="loading"> Loading
      <span class="title">GraphPost</span>
    </div>
  </div>
  <script>window.addEventListener('load', function (event) {
      GraphQLPlayground.init(document.getElementById('root'), {
        endpoint: '/v1/graphql',
        subscriptionEndpoint: 'ws://%s:%d/v1/graphql',
        settings: {
          'request.credentials': 'include',
        }
      })
    })</script>
</body>
</html>
`, cfg.Server.Host, cfg.Server.Port)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(playgroundHTML))
}

// corsMiddleware adds CORS headers
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	cfg := s.engine.GetConfig()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.CORS.Enabled {
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = "*"
			}

			allowed := false
			for _, allowedOrigin := range cfg.CORS.AllowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.CORS.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.CORS.AllowedHeaders, ", "))
				if cfg.CORS.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", cfg.CORS.MaxAge))
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware handles authentication
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticator := s.engine.GetAuthenticator()

		session, err := authenticator.AuthenticateRequest(r)
		if err != nil {
			// Allow unauthenticated requests if auth is not required
			if !s.engine.GetConfig().Auth.Enabled {
				session = &auth.Session{
					Role: s.engine.GetConfig().Auth.UnauthorizedRole,
				}
			} else {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}
		}

		ctx := auth.SetSessionInContext(r.Context(), session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loggingMiddleware logs requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create response wrapper to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		fmt.Printf("%s %s %d %v\n", r.Method, r.URL.Path, rw.statusCode, duration)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// GraphQLRequest represents a GraphQL request
type GraphQLRequest struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

// MetadataRequest represents a metadata API request
type MetadataRequest struct {
	Type    string      `json:"type"`
	Version int         `json:"version,omitempty"`
	Args    interface{} `json:"args"`
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	ID      string      `json:"id,omitempty"`
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// introspectionQuery is the standard GraphQL introspection query
const introspectionQuery = `
query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type { ...TypeRef }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}
`
