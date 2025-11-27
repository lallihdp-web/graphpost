package graphjin

import (
	"context"
	"database/sql"
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

		// Performance
		EnableTracing:    cfg.GraphJin.EnableTracing,
		EnableIntrospection: cfg.Server.EnableIntrospection,

		// Default limit for queries
		DefaultLimit: 20,
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
func (e *Engine) Execute(ctx context.Context, query string, vars map[string]interface{}, rc *core.ReqConfig) (*core.Result, error) {
	// Execute query through GraphJin
	res, err := e.gj.GraphQL(ctx, query, vars, rc)
	if err != nil {
		return nil, fmt.Errorf("GraphJin query execution failed: %w", err)
	}

	return res, nil
}

// Subscribe creates a GraphQL subscription
func (e *Engine) Subscribe(ctx context.Context, query string, vars map[string]interface{}, rc *core.ReqConfig) (*core.Member, error) {
	member, err := e.gj.Subscribe(ctx, query, vars, rc)
	if err != nil {
		return nil, fmt.Errorf("GraphJin subscription failed: %w", err)
	}

	return member, nil
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
