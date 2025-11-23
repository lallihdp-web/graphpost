package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/graphpost/graphpost/internal/auth"
	"github.com/graphpost/graphpost/internal/config"
	"github.com/graphpost/graphpost/internal/database"
	"github.com/graphpost/graphpost/internal/events"
	"github.com/graphpost/graphpost/internal/resolver"
	"github.com/graphpost/graphpost/internal/schema"
	"github.com/graphpost/graphpost/internal/subscription"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/gqlerrors"
)

// Engine is the main GraphQL engine
type Engine struct {
	config         *config.Config
	dbConn         *database.Connection
	dbSchema       *database.Schema
	graphqlSchema  *graphql.Schema
	resolver       *resolver.Resolver
	authenticator  *auth.Authenticator
	subManager     *subscription.Manager
	triggerManager *events.TriggerManager
	mu             sync.RWMutex
	ready          bool
}

// NewEngine creates a new GraphQL engine
func NewEngine(cfg *config.Config) *Engine {
	return &Engine{
		config: cfg,
	}
}

// Initialize initializes the engine
func (e *Engine) Initialize(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Connect to database
	dbConn, err := database.NewConnection(&e.config.Database)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	e.dbConn = dbConn

	// Introspect database schema
	introspector := database.NewIntrospector(dbConn.Pool(), e.config.Database.Schema)
	dbSchema, err := introspector.IntrospectSchema(ctx)
	if err != nil {
		return fmt.Errorf("failed to introspect database schema: %w", err)
	}
	e.dbSchema = dbSchema

	// Create resolver
	e.resolver = resolver.NewResolver(dbConn.Pool(), dbSchema, e.config.Database.Schema)

	// Generate GraphQL schema
	generator := schema.NewGenerator(dbSchema, &e.config.GraphQL)
	graphqlSchema, err := generator.Generate()
	if err != nil {
		return fmt.Errorf("failed to generate GraphQL schema: %w", err)
	}
	e.graphqlSchema = graphqlSchema

	// Initialize authenticator
	e.authenticator = auth.NewAuthenticator(&e.config.Auth)

	// Initialize subscription manager
	e.subManager = subscription.NewManager(dbConn, e.resolver, dbSchema)

	// Initialize event trigger manager
	e.triggerManager = events.NewTriggerManager(dbConn, &e.config.Events)

	e.ready = true
	return nil
}

// Start starts the engine
func (e *Engine) Start(ctx context.Context) error {
	// Start subscription manager
	if err := e.subManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start subscription manager: %w", err)
	}

	// Start event trigger manager
	if e.config.Events.Enabled {
		e.triggerManager.Start(ctx)
	}

	return nil
}

// Stop stops the engine with graceful shutdown
func (e *Engine) Stop(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var errs []error

	// Stop subscription manager first (stops accepting new subscriptions)
	if e.subManager != nil {
		if err := e.subManager.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("subscription manager: %w", err))
		}
	}

	// Stop event trigger manager
	if e.triggerManager != nil {
		e.triggerManager.Stop()
	}

	// Gracefully close database connection (waits for active queries)
	if e.dbConn != nil {
		if err := e.dbConn.GracefulClose(ctx); err != nil {
			errs = append(errs, fmt.Errorf("database: %w", err))
		}
	}

	e.ready = false

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}

// Name returns the component name for shutdown manager
func (e *Engine) Name() string {
	return "engine"
}

// Shutdown implements the ShutdownComponent interface
func (e *Engine) Shutdown(ctx context.Context) error {
	return e.Stop(ctx)
}

// GetDBConnection returns the database connection
func (e *Engine) GetDBConnection() *database.Connection {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.dbConn
}

// ExecuteQuery executes a GraphQL query
func (e *Engine) ExecuteQuery(ctx context.Context, query string, variables map[string]interface{}, operationName string) *graphql.Result {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.ready {
		return &graphql.Result{
			Errors: []gqlerrors.FormattedError{
				{Message: "engine not ready"},
			},
		}
	}

	result := graphql.Do(graphql.Params{
		Schema:         *e.graphqlSchema,
		RequestString:  query,
		VariableValues: variables,
		OperationName:  operationName,
		Context:        ctx,
	})

	return result
}

// Reload reloads the engine (schema refresh)
func (e *Engine) Reload(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Re-introspect database schema
	introspector := database.NewIntrospector(e.dbConn.Pool(), e.config.Database.Schema)
	dbSchema, err := introspector.IntrospectSchema(ctx)
	if err != nil {
		return fmt.Errorf("failed to introspect database schema: %w", err)
	}
	e.dbSchema = dbSchema

	// Recreate resolver
	e.resolver = resolver.NewResolver(e.dbConn.Pool(), dbSchema, e.config.Database.Schema)

	// Regenerate GraphQL schema
	generator := schema.NewGenerator(dbSchema, &e.config.GraphQL)
	graphqlSchema, err := generator.Generate()
	if err != nil {
		return fmt.Errorf("failed to generate GraphQL schema: %w", err)
	}
	e.graphqlSchema = graphqlSchema

	return nil
}

// GetSchema returns the GraphQL schema
func (e *Engine) GetSchema() *graphql.Schema {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.graphqlSchema
}

// GetDBSchema returns the database schema
func (e *Engine) GetDBSchema() *database.Schema {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.dbSchema
}

// GetResolver returns the resolver
func (e *Engine) GetResolver() *resolver.Resolver {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.resolver
}

// GetAuthenticator returns the authenticator
func (e *Engine) GetAuthenticator() *auth.Authenticator {
	return e.authenticator
}

// GetSubscriptionManager returns the subscription manager
func (e *Engine) GetSubscriptionManager() *subscription.Manager {
	return e.subManager
}

// GetTriggerManager returns the event trigger manager
func (e *Engine) GetTriggerManager() *events.TriggerManager {
	return e.triggerManager
}

// GetConfig returns the configuration
func (e *Engine) GetConfig() *config.Config {
	return e.config
}

// IsReady returns whether the engine is ready
func (e *Engine) IsReady() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ready
}

// Health returns the health status
func (e *Engine) Health(ctx context.Context) map[string]interface{} {
	health := map[string]interface{}{
		"status": "ok",
	}

	// Check database connection
	if e.dbConn != nil {
		if err := e.dbConn.Ping(ctx); err != nil {
			health["status"] = "unhealthy"
			health["database"] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
		} else {
			health["database"] = map[string]interface{}{
				"status": "healthy",
			}
		}
	}

	return health
}

// gqlerror is a simple GraphQL error
type gqlerror struct {
	Message string `json:"message"`
}

func (e gqlerror) Error() string {
	return e.Message
}

// Metadata holds engine metadata
type Metadata struct {
	Tables         map[string]*TableMetadata         `json:"tables"`
	RemoteSchemas  map[string]*RemoteSchemaMetadata  `json:"remote_schemas"`
	Actions        map[string]*ActionMetadata        `json:"actions"`
	CustomTypes    *CustomTypesMetadata              `json:"custom_types"`
	EventTriggers  map[string]*events.EventTrigger   `json:"event_triggers"`
}

// TableMetadata holds table metadata
type TableMetadata struct {
	Name              string                        `json:"name"`
	Schema            string                        `json:"schema"`
	Configuration     *TableConfiguration           `json:"configuration"`
	ObjectRelationships []*RelationshipMetadata     `json:"object_relationships"`
	ArrayRelationships  []*RelationshipMetadata     `json:"array_relationships"`
	SelectPermissions   []*auth.PermissionRule      `json:"select_permissions"`
	InsertPermissions   []*auth.PermissionRule      `json:"insert_permissions"`
	UpdatePermissions   []*auth.PermissionRule      `json:"update_permissions"`
	DeletePermissions   []*auth.PermissionRule      `json:"delete_permissions"`
	ComputedFields      []*ComputedFieldMetadata    `json:"computed_fields"`
}

// TableConfiguration holds table configuration
type TableConfiguration struct {
	CustomRootFields *CustomRootFields            `json:"custom_root_fields"`
	CustomColumnNames map[string]string           `json:"custom_column_names"`
	Comment          string                       `json:"comment"`
}

// CustomRootFields holds custom root field names
type CustomRootFields struct {
	Select          string `json:"select,omitempty"`
	SelectByPK      string `json:"select_by_pk,omitempty"`
	SelectAggregate string `json:"select_aggregate,omitempty"`
	Insert          string `json:"insert,omitempty"`
	InsertOne       string `json:"insert_one,omitempty"`
	Update          string `json:"update,omitempty"`
	UpdateByPK      string `json:"update_by_pk,omitempty"`
	Delete          string `json:"delete,omitempty"`
	DeleteByPK      string `json:"delete_by_pk,omitempty"`
}

// RelationshipMetadata holds relationship metadata
type RelationshipMetadata struct {
	Name    string                 `json:"name"`
	Using   map[string]interface{} `json:"using"`
	Comment string                 `json:"comment,omitempty"`
}

// ComputedFieldMetadata holds computed field metadata
type ComputedFieldMetadata struct {
	Name       string                     `json:"name"`
	Definition ComputedFieldDefinition    `json:"definition"`
	Comment    string                     `json:"comment,omitempty"`
}

// ComputedFieldDefinition defines a computed field
type ComputedFieldDefinition struct {
	Function     FunctionReference `json:"function"`
	TableArgument string           `json:"table_argument,omitempty"`
	SessionArgument string         `json:"session_argument,omitempty"`
}

// FunctionReference references a database function
type FunctionReference struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
}

// RemoteSchemaMetadata holds remote schema metadata
type RemoteSchemaMetadata struct {
	Name       string                 `json:"name"`
	Definition RemoteSchemaDefinition `json:"definition"`
	Comment    string                 `json:"comment,omitempty"`
}

// RemoteSchemaDefinition defines a remote schema
type RemoteSchemaDefinition struct {
	URL                 string            `json:"url"`
	Headers             map[string]string `json:"headers,omitempty"`
	ForwardClientHeaders bool             `json:"forward_client_headers,omitempty"`
	TimeoutSeconds      int               `json:"timeout_seconds,omitempty"`
}

// ActionMetadata holds action metadata
type ActionMetadata struct {
	Name       string           `json:"name"`
	Definition ActionDefinition `json:"definition"`
	Comment    string           `json:"comment,omitempty"`
}

// ActionDefinition defines an action
type ActionDefinition struct {
	Handler     string                   `json:"handler"`
	Type        string                   `json:"type"` // mutation or query
	Kind        string                   `json:"kind"` // synchronous or asynchronous
	Headers     []ActionHeader           `json:"headers,omitempty"`
	Arguments   []ActionArgument         `json:"arguments,omitempty"`
	OutputType  string                   `json:"output_type"`
}

// ActionHeader defines an action header
type ActionHeader struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	ValueFromEnv string `json:"value_from_env,omitempty"`
}

// ActionArgument defines an action argument
type ActionArgument struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CustomTypesMetadata holds custom types metadata
type CustomTypesMetadata struct {
	InputObjects []CustomInputObject `json:"input_objects,omitempty"`
	Objects      []CustomObject      `json:"objects,omitempty"`
	Scalars      []CustomScalar      `json:"scalars,omitempty"`
	Enums        []CustomEnum        `json:"enums,omitempty"`
}

// CustomInputObject defines a custom input object type
type CustomInputObject struct {
	Name   string         `json:"name"`
	Fields []CustomField  `json:"fields"`
}

// CustomObject defines a custom object type
type CustomObject struct {
	Name   string         `json:"name"`
	Fields []CustomField  `json:"fields"`
}

// CustomField defines a custom field
type CustomField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CustomScalar defines a custom scalar type
type CustomScalar struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CustomEnum defines a custom enum type
type CustomEnum struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}
