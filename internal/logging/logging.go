package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Config holds logging configuration
type Config struct {
	// Level is the minimum log level (debug, info, warn, error)
	Level string `json:"level"`

	// Format is the log format (json, text)
	Format string `json:"format"`

	// Output is where logs are written (stdout, stderr, or file path)
	Output string `json:"output"`

	// QueryLog enables SQL query logging
	QueryLog bool `json:"query_log"`

	// QueryLogLevel is the level for query logs (debug, info)
	QueryLogLevel string `json:"query_log_level"`

	// SlowQueryThreshold is the duration above which queries are logged as slow
	// Set to 0 to disable slow query logging
	SlowQueryThreshold time.Duration `json:"slow_query_threshold"`

	// SlowQueryLogLevel is the level for slow query logs (warn, error)
	SlowQueryLogLevel string `json:"slow_query_log_level"`

	// LogQueryParams includes query parameters in logs (be careful with sensitive data)
	LogQueryParams bool `json:"log_query_params"`

	// RequestLog enables HTTP request logging
	RequestLog bool `json:"request_log"`

	// IncludeStackTrace includes stack traces for errors
	IncludeStackTrace bool `json:"include_stack_trace"`
}

// DefaultConfig returns default logging configuration
func DefaultConfig() Config {
	return Config{
		Level:              "info",
		Format:             "json",
		Output:             "stdout",
		QueryLog:           false,
		QueryLogLevel:      "debug",
		SlowQueryThreshold: 1 * time.Second,
		SlowQueryLogLevel:  "warn",
		LogQueryParams:     false,
		RequestLog:         true,
		IncludeStackTrace:  false,
	}
}

// Level represents log levels
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "info"
	}
}

func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Logger is the main logging interface
type Logger struct {
	config Config
	level  Level
	output io.Writer
	mu     sync.Mutex
}

var (
	globalLogger *Logger
	loggerOnce   sync.Once
)

// Initialize initializes the global logger
func Initialize(cfg Config) (*Logger, error) {
	var err error
	loggerOnce.Do(func() {
		globalLogger, err = NewLogger(cfg)
	})
	return globalLogger, err
}

// Get returns the global logger instance
func Get() *Logger {
	if globalLogger == nil {
		globalLogger, _ = NewLogger(DefaultConfig())
	}
	return globalLogger
}

// NewLogger creates a new logger
func NewLogger(cfg Config) (*Logger, error) {
	var output io.Writer
	switch cfg.Output {
	case "stdout", "":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:
		f, err := os.OpenFile(cfg.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		output = f
	}

	return &Logger{
		config: cfg,
		level:  ParseLevel(cfg.Level),
		output: output,
	}, nil
}

// LogEntry represents a log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func (l *Logger) log(level Level, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level.String(),
		Message:   msg,
		Fields:    fields,
	}

	if l.config.Format == "json" {
		data, _ := json.Marshal(entry)
		fmt.Fprintln(l.output, string(data))
	} else {
		// Text format
		var fieldsStr string
		if len(fields) > 0 {
			parts := make([]string, 0, len(fields))
			for k, v := range fields {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
			fieldsStr = " " + strings.Join(parts, " ")
		}
		fmt.Fprintf(l.output, "%s [%s] %s%s\n", entry.Timestamp, strings.ToUpper(entry.Level), msg, fieldsStr)
	}
}

// Debug logs at debug level
func (l *Logger) Debug(msg string, fields map[string]interface{}) {
	l.log(LevelDebug, msg, fields)
}

// Info logs at info level
func (l *Logger) Info(msg string, fields map[string]interface{}) {
	l.log(LevelInfo, msg, fields)
}

// Warn logs at warn level
func (l *Logger) Warn(msg string, fields map[string]interface{}) {
	l.log(LevelWarn, msg, fields)
}

// Error logs at error level
func (l *Logger) Error(msg string, fields map[string]interface{}) {
	l.log(LevelError, msg, fields)
}

// QueryLog logs a database query
type QueryLog struct {
	Query      string
	Args       []interface{}
	Duration   time.Duration
	RowCount   int64
	Error      error
	Table      string
	Operation  string
	ctx        context.Context
}

// LogQuery logs a database query
func (l *Logger) LogQuery(q QueryLog) {
	if l == nil || !l.config.QueryLog {
		return
	}

	fields := map[string]interface{}{
		"query_type": "sql",
		"duration":   q.Duration.String(),
		"duration_ms": q.Duration.Milliseconds(),
	}

	if q.Table != "" {
		fields["table"] = q.Table
	}
	if q.Operation != "" {
		fields["operation"] = q.Operation
	}
	if q.RowCount >= 0 {
		fields["row_count"] = q.RowCount
	}

	// Include query (truncate if too long)
	query := q.Query
	if len(query) > 1000 {
		query = query[:1000] + "..."
	}
	fields["query"] = query

	// Include params if configured
	if l.config.LogQueryParams && len(q.Args) > 0 {
		fields["params"] = sanitizeParams(q.Args)
	}

	if q.Error != nil {
		fields["error"] = q.Error.Error()
		l.log(LevelError, "Query failed", fields)
		return
	}

	// Check for slow query
	if l.config.SlowQueryThreshold > 0 && q.Duration >= l.config.SlowQueryThreshold {
		fields["slow_query"] = true
		fields["threshold"] = l.config.SlowQueryThreshold.String()
		l.log(ParseLevel(l.config.SlowQueryLogLevel), "Slow query detected", fields)
		return
	}

	// Regular query log
	l.log(ParseLevel(l.config.QueryLogLevel), "Query executed", fields)
}

// SlowQueryInfo contains information about slow queries for optimization
type SlowQueryInfo struct {
	Query           string        `json:"query"`
	Table           string        `json:"table"`
	Operation       string        `json:"operation"`
	Duration        time.Duration `json:"duration"`
	Suggestion      string        `json:"suggestion"`
}

// AnalyzeSlowQuery analyzes a slow query and provides optimization suggestions
func AnalyzeSlowQuery(query string, table string, duration time.Duration) SlowQueryInfo {
	info := SlowQueryInfo{
		Query:     query,
		Table:     table,
		Duration:  duration,
	}

	query = strings.ToUpper(query)

	// Detect operation type
	switch {
	case strings.HasPrefix(query, "SELECT"):
		info.Operation = "SELECT"
	case strings.HasPrefix(query, "INSERT"):
		info.Operation = "INSERT"
	case strings.HasPrefix(query, "UPDATE"):
		info.Operation = "UPDATE"
	case strings.HasPrefix(query, "DELETE"):
		info.Operation = "DELETE"
	}

	// Provide suggestions based on query patterns
	var suggestions []string

	// Check for missing WHERE clause on UPDATE/DELETE
	if (info.Operation == "UPDATE" || info.Operation == "DELETE") && !strings.Contains(query, "WHERE") {
		suggestions = append(suggestions, "WARNING: No WHERE clause - may affect all rows")
	}

	// Check for SELECT *
	if strings.Contains(query, "SELECT *") {
		suggestions = append(suggestions, "Consider selecting specific columns instead of SELECT *")
	}

	// Check for LIKE with leading wildcard
	if strings.Contains(query, "LIKE '%") {
		suggestions = append(suggestions, "LIKE with leading wildcard prevents index usage - consider full-text search")
	}

	// Check for ORDER BY without index hints
	if strings.Contains(query, "ORDER BY") && !strings.Contains(query, "LIMIT") {
		suggestions = append(suggestions, "ORDER BY without LIMIT may be slow - consider adding LIMIT or index on sort column")
	}

	// Check for JOIN without conditions
	if strings.Contains(query, "JOIN") && !strings.Contains(query, "ON ") {
		suggestions = append(suggestions, "JOIN without ON clause - ensure proper join conditions")
	}

	// Check for aggregate functions
	if strings.Contains(query, "COUNT(") || strings.Contains(query, "SUM(") ||
		strings.Contains(query, "AVG(") || strings.Contains(query, "MAX(") || strings.Contains(query, "MIN(") {
		if !strings.Contains(query, "GROUP BY") {
			suggestions = append(suggestions, "Aggregate function on large table - consider materialized views for frequent queries")
		} else {
			suggestions = append(suggestions, "GROUP BY query - ensure index exists on grouping columns")
		}
	}

	// Check for subqueries
	if strings.Count(query, "SELECT") > 1 {
		suggestions = append(suggestions, "Subquery detected - consider using JOINs or CTEs for better performance")
	}

	// Duration-based suggestions
	if duration > 5*time.Second {
		suggestions = append(suggestions, "Query very slow (>5s) - consider: 1) Adding indexes 2) Query rewrite 3) Table partitioning")
	} else if duration > 1*time.Second {
		suggestions = append(suggestions, "Query slow (>1s) - review execution plan with EXPLAIN ANALYZE")
	}

	// Table-specific suggestions
	if table != "" {
		suggestions = append(suggestions, fmt.Sprintf("Check indexes on table '%s' with: \\d %s", table, table))
		suggestions = append(suggestions, fmt.Sprintf("Analyze table statistics: ANALYZE %s", table))
	}

	info.Suggestion = strings.Join(suggestions, "; ")
	return info
}

// LogSlowQueryAnalysis logs analysis of a slow query
func (l *Logger) LogSlowQueryAnalysis(q QueryLog) {
	if l == nil || !l.config.QueryLog {
		return
	}

	if l.config.SlowQueryThreshold > 0 && q.Duration >= l.config.SlowQueryThreshold {
		analysis := AnalyzeSlowQuery(q.Query, q.Table, q.Duration)

		fields := map[string]interface{}{
			"query":       analysis.Query,
			"table":       analysis.Table,
			"operation":   analysis.Operation,
			"duration":    analysis.Duration.String(),
			"duration_ms": analysis.Duration.Milliseconds(),
			"suggestions": analysis.Suggestion,
		}

		l.log(LevelWarn, "Slow query analysis", fields)
	}
}

// RequestLog logs an HTTP request
type RequestLog struct {
	Method        string
	Path          string
	StatusCode    int
	Duration      time.Duration
	ClientIP      string
	UserAgent     string
	OperationType string
	OperationName string
	Error         error
}

// LogRequest logs an HTTP request
func (l *Logger) LogRequest(r RequestLog) {
	if l == nil || !l.config.RequestLog {
		return
	}

	fields := map[string]interface{}{
		"method":      r.Method,
		"path":        r.Path,
		"status":      r.StatusCode,
		"duration":    r.Duration.String(),
		"duration_ms": r.Duration.Milliseconds(),
	}

	if r.ClientIP != "" {
		fields["client_ip"] = r.ClientIP
	}
	if r.UserAgent != "" {
		fields["user_agent"] = r.UserAgent
	}
	if r.OperationType != "" {
		fields["graphql_operation"] = r.OperationType
	}
	if r.OperationName != "" {
		fields["graphql_operation_name"] = r.OperationName
	}

	if r.Error != nil {
		fields["error"] = r.Error.Error()
		l.log(LevelError, "Request failed", fields)
		return
	}

	level := LevelInfo
	if r.StatusCode >= 500 {
		level = LevelError
	} else if r.StatusCode >= 400 {
		level = LevelWarn
	}

	l.log(level, "Request completed", fields)
}

// sanitizeParams sanitizes query parameters for logging
func sanitizeParams(args []interface{}) []interface{} {
	sanitized := make([]interface{}, len(args))
	for i, arg := range args {
		// Truncate long strings
		if s, ok := arg.(string); ok && len(s) > 100 {
			sanitized[i] = s[:100] + "..."
		} else {
			sanitized[i] = arg
		}
	}
	return sanitized
}

// Config returns the logger configuration
func (l *Logger) Config() Config {
	if l == nil {
		return DefaultConfig()
	}
	return l.config
}

// QueryLogEnabled returns whether query logging is enabled
func (l *Logger) QueryLogEnabled() bool {
	return l != nil && l.config.QueryLog
}

// SlowQueryThreshold returns the slow query threshold
func (l *Logger) SlowQueryThreshold() time.Duration {
	if l == nil {
		return 0
	}
	return l.config.SlowQueryThreshold
}
