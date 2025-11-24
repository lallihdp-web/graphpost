package telemetry

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds telemetry configuration
type Config struct {
	// Enabled enables OpenTelemetry integration
	Enabled bool `json:"enabled"`

	// ServiceName is the name of this service in traces
	ServiceName string `json:"service_name"`

	// ServiceVersion is the version of this service
	ServiceVersion string `json:"service_version"`

	// OTLPEndpoint is the OpenTelemetry collector endpoint
	// Example: "localhost:4317" for gRPC, "localhost:4318" for HTTP
	OTLPEndpoint string `json:"otlp_endpoint"`

	// OTLPProtocol is the protocol to use: "grpc" or "http"
	OTLPProtocol string `json:"otlp_protocol"`

	// OTLPInsecure disables TLS for the OTLP connection
	OTLPInsecure bool `json:"otlp_insecure"`

	// SampleRate is the sampling rate for traces (0.0 to 1.0)
	// 1.0 = sample all traces, 0.1 = sample 10% of traces
	SampleRate float64 `json:"sample_rate"`

	// TraceQueries enables tracing for individual database queries
	TraceQueries bool `json:"trace_queries"`

	// TraceResolvers enables tracing for GraphQL resolvers
	TraceResolvers bool `json:"trace_resolvers"`
}

// DefaultConfig returns default telemetry configuration
func DefaultConfig() Config {
	return Config{
		Enabled:        false,
		ServiceName:    "graphpost",
		ServiceVersion: "1.0.0",
		OTLPEndpoint:   "localhost:4317",
		OTLPProtocol:   "grpc",
		OTLPInsecure:   true,
		SampleRate:     1.0,
		TraceQueries:   true,
		TraceResolvers: true,
	}
}

// Telemetry manages OpenTelemetry tracing and metrics
type Telemetry struct {
	config         Config
	tracerProvider *sdktrace.TracerProvider
	tracer         trace.Tracer
	meter          metric.Meter
	mu             sync.RWMutex

	// Metrics
	requestCounter     metric.Int64Counter
	requestDuration    metric.Float64Histogram
	queryCounter       metric.Int64Counter
	queryDuration      metric.Float64Histogram
	activeConnections  metric.Int64UpDownCounter
	poolConnections    metric.Int64ObservableGauge
	errorCounter       metric.Int64Counter
}

var (
	globalTelemetry *Telemetry
	once            sync.Once
)

// Initialize initializes the global telemetry instance
func Initialize(cfg Config) (*Telemetry, error) {
	var err error
	once.Do(func() {
		globalTelemetry, err = newTelemetry(cfg)
	})
	return globalTelemetry, err
}

// Get returns the global telemetry instance
func Get() *Telemetry {
	return globalTelemetry
}

func newTelemetry(cfg Config) (*Telemetry, error) {
	t := &Telemetry{
		config: cfg,
	}

	if !cfg.Enabled {
		// Create no-op tracer for disabled state
		t.tracer = otel.Tracer(cfg.ServiceName)
		return t, nil
	}

	ctx := context.Background()

	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("deployment.environment", "production"),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create OTLP exporter
	var exporter *otlptrace.Exporter
	if cfg.OTLPProtocol == "http" {
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		exporter, err = otlptracehttp.New(ctx, opts...)
	} else {
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		exporter, err = otlptracegrpc.New(ctx, opts...)
	}
	if err != nil {
		return nil, err
	}

	// Create sampler
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create tracer provider
	t.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(t.tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create tracer
	t.tracer = t.tracerProvider.Tracer(cfg.ServiceName)

	return t, nil
}

// Shutdown gracefully shuts down telemetry
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t.tracerProvider != nil {
		return t.tracerProvider.Shutdown(ctx)
	}
	return nil
}

// Tracer returns the tracer instance
func (t *Telemetry) Tracer() trace.Tracer {
	if t == nil {
		return otel.Tracer("graphpost")
	}
	return t.tracer
}

// StartSpan starts a new span
func (t *Telemetry) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if t == nil || t.tracer == nil {
		return ctx, nil
	}
	return t.tracer.Start(ctx, name, opts...)
}

// StartGraphQLSpan starts a span for a GraphQL operation
func (t *Telemetry) StartGraphQLSpan(ctx context.Context, operationType, operationName string) (context.Context, trace.Span) {
	if t == nil || !t.config.Enabled {
		return ctx, nil
	}

	return t.tracer.Start(ctx, "graphql."+operationType,
		trace.WithAttributes(
			attribute.String("graphql.operation.type", operationType),
			attribute.String("graphql.operation.name", operationName),
		),
		trace.WithSpanKind(trace.SpanKindServer),
	)
}

// StartQuerySpan starts a span for a database query
func (t *Telemetry) StartQuerySpan(ctx context.Context, queryType, tableName string) (context.Context, trace.Span) {
	if t == nil || !t.config.Enabled || !t.config.TraceQueries {
		return ctx, nil
	}

	return t.tracer.Start(ctx, "db.query",
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", queryType),
			attribute.String("db.sql.table", tableName),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// StartResolverSpan starts a span for a GraphQL resolver
func (t *Telemetry) StartResolverSpan(ctx context.Context, fieldName, parentType string) (context.Context, trace.Span) {
	if t == nil || !t.config.Enabled || !t.config.TraceResolvers {
		return ctx, nil
	}

	return t.tracer.Start(ctx, "graphql.resolve",
		trace.WithAttributes(
			attribute.String("graphql.field.name", fieldName),
			attribute.String("graphql.field.type", parentType),
		),
	)
}

// RecordError records an error on a span
func RecordError(span trace.Span, err error) {
	if span != nil && err != nil {
		span.RecordError(err)
	}
}

// EndSpan ends a span safely
func EndSpan(span trace.Span) {
	if span != nil {
		span.End()
	}
}

// SetSpanAttributes sets attributes on a span
func SetSpanAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	if span != nil {
		span.SetAttributes(attrs...)
	}
}

// SpanFromContext returns the current span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// ContextWithSpan returns a new context with the span
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

// QueryMetrics records query metrics
type QueryMetrics struct {
	StartTime time.Time
	QueryType string
	TableName string
	RowCount  int64
	Error     error
}

// RecordQueryMetrics records metrics for a query
func (t *Telemetry) RecordQueryMetrics(ctx context.Context, m QueryMetrics) {
	if t == nil || !t.config.Enabled {
		return
	}

	duration := time.Since(m.StartTime).Seconds()
	attrs := []attribute.KeyValue{
		attribute.String("query_type", m.QueryType),
		attribute.String("table", m.TableName),
	}

	if m.Error != nil {
		attrs = append(attrs, attribute.Bool("error", true))
	}

	// Record duration histogram
	if t.queryDuration != nil {
		t.queryDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
	}

	// Increment counter
	if t.queryCounter != nil {
		t.queryCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

// RequestMetrics records HTTP request metrics
type RequestMetrics struct {
	StartTime     time.Time
	Method        string
	Path          string
	StatusCode    int
	OperationType string
	OperationName string
	Error         error
}

// RecordRequestMetrics records metrics for an HTTP request
func (t *Telemetry) RecordRequestMetrics(ctx context.Context, m RequestMetrics) {
	if t == nil || !t.config.Enabled {
		return
	}

	duration := time.Since(m.StartTime).Seconds()
	attrs := []attribute.KeyValue{
		attribute.String("http.method", m.Method),
		attribute.String("http.route", m.Path),
		attribute.Int("http.status_code", m.StatusCode),
	}

	if m.OperationType != "" {
		attrs = append(attrs, attribute.String("graphql.operation.type", m.OperationType))
	}
	if m.OperationName != "" {
		attrs = append(attrs, attribute.String("graphql.operation.name", m.OperationName))
	}

	// Record duration histogram
	if t.requestDuration != nil {
		t.requestDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
	}

	// Increment counter
	if t.requestCounter != nil {
		t.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	}

	// Record errors
	if m.Error != nil && t.errorCounter != nil {
		t.errorCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("error.type", "request"),
		))
	}
}

// IsEnabled returns whether telemetry is enabled
func (t *Telemetry) IsEnabled() bool {
	if t == nil {
		return false
	}
	return t.config.Enabled
}

// Config returns the telemetry configuration
func (t *Telemetry) Config() Config {
	if t == nil {
		return DefaultConfig()
	}
	return t.config
}
