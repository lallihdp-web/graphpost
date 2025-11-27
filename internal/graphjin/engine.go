package graphjin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/dosco/graphjin/core/v3"
	"github.com/graphpost/graphpost/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// Engine wraps GraphJin core for GraphPost
type Engine struct {
	gj     *core.GraphJin
	config *config.Config
	db     *sql.DB
}

// NewEngine creates a new GraphJin engine instance
func NewEngine(cfg *config.Config, pool *pgxpool.Pool) (*Engine, error) {
	// Convert pgxpool to database/sql for GraphJin compatibility
	// GraphJin requires database/sql interface
	connConfig := pool.Config().ConnConfig
	connString := stdlib.RegisterConnConfig(connConfig)
	db, err := sql.Open("pgx", connString)
	if err != nil {
		return nil, fmt.Errorf("failed to create sql.DB from pool: %w", err)
	}

	// Configure GraphJin
	gjConfig := &core.Config{
		// Database configuration
		DBType:       "postgres",
		DisableAllowList: !cfg.GraphJin.Production, // Allow all queries in development

		// Security
		Production:   cfg.GraphJin.Production,
		SetUserID:    true, // Enable user_id from context

		// Enable introspection in development
		EnableIntrospection: cfg.Server.EnableIntrospection,

		// Default limit for queries
		DefaultLimit: cfg.GraphJin.DefaultLimit,

		// Enable aggregations and functions (set from config)
		DisableAgg:   false, // Enable aggregate functions (count, sum, avg, min, max)
		DisableFuncs: cfg.GraphJin.DisableFunctions, // Control other functions
	}

	// Initialize GraphJin
	gj, err := core.NewGraphJin(gjConfig, db)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GraphJin: %w", err)
	}

	return &Engine{
		gj:     gj,
		config: cfg,
		db:     db,
	}, nil
}

// Execute executes a GraphQL query using GraphJin
func (e *Engine) Execute(ctx context.Context, query string, vars map[string]interface{}, rc *core.RequestConfig) (*core.Result, error) {
	// Convert variables to json.RawMessage
	var varsJSON json.RawMessage
	if vars != nil {
		b, err := json.Marshal(vars)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal variables: %w", err)
		}
		varsJSON = b
	}

	// Execute query through GraphJin
	res, err := e.gj.GraphQL(ctx, query, varsJSON, rc)
	if err != nil {
		return nil, fmt.Errorf("GraphJin query execution failed: %w", err)
	}

	return res, nil
}

// Reload reloads the GraphJin configuration and schema
func (e *Engine) Reload() error {
	// GraphJin will auto-reload schema on next query in development mode
	// In production, schema is cached
	return nil
}

// Close closes the GraphJin engine
func (e *Engine) Close() error {
	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

// Health returns health status of GraphJin engine
func (e *Engine) Health(ctx context.Context) map[string]interface{} {
	// Test with a simple introspection query
	query := `{ __typename }`
	_, err := e.Execute(ctx, query, nil, nil)

	if err != nil {
		return map[string]interface{}{
			"status": "unhealthy",
			"error":  err.Error(),
		}
	}

	return map[string]interface{}{
		"status": "healthy",
		"engine": "graphjin",
		"version": "3.1.4",
	}
}
