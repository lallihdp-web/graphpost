package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/graphpost/graphpost/internal/config"
	"github.com/graphpost/graphpost/internal/database"
)

// TriggerManager manages event triggers
type TriggerManager struct {
	db          *database.Connection
	config      *config.EventsConfig
	triggers    map[string]*EventTrigger
	mu          sync.RWMutex
	httpClient  *http.Client
	workerPool  chan struct{}
	eventQueue  chan *Event
	stopChan    chan struct{}
}

// EventTrigger represents an event trigger configuration
type EventTrigger struct {
	Name          string            `json:"name"`
	Table         string            `json:"table"`
	Schema        string            `json:"schema"`
	Operations    []string          `json:"operations"` // INSERT, UPDATE, DELETE
	WebhookURL    string            `json:"webhook_url"`
	Headers       map[string]string `json:"headers"`
	RetryConfig   *RetryConfig      `json:"retry_config"`
	EnableManual  bool              `json:"enable_manual"`
	ReplaceSets   []ReplaceSet      `json:"replace_sets,omitempty"`
	Enabled       bool              `json:"enabled"`
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	NumRetries      int   `json:"num_retries"`
	RetryInterval   int   `json:"retry_interval_seconds"`
	TimeoutSeconds  int   `json:"timeout_seconds"`
	ToleranceSeconds int  `json:"tolerance_seconds"`
}

// ReplaceSet defines column replacements for updates
type ReplaceSet struct {
	Column string `json:"column"`
}

// Event represents a triggered event
type Event struct {
	ID            string                 `json:"id"`
	TriggerName   string                 `json:"trigger_name"`
	Table         TableInfo              `json:"table"`
	Operation     string                 `json:"operation"`
	NewData       map[string]interface{} `json:"new_data,omitempty"`
	OldData       map[string]interface{} `json:"old_data,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	DeliveryInfo  *DeliveryInfo          `json:"delivery_info,omitempty"`
	RetryCount    int                    `json:"retry_count"`
	Status        EventStatus            `json:"status"`
}

// TableInfo contains table information
type TableInfo struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

// DeliveryInfo contains event delivery information
type DeliveryInfo struct {
	CurrentRetry    int        `json:"current_retry"`
	MaxRetries      int        `json:"max_retries"`
	DeliveredAt     *time.Time `json:"delivered_at,omitempty"`
	ResponseStatus  int        `json:"response_status,omitempty"`
	ResponseBody    string     `json:"response_body,omitempty"`
	Error           string     `json:"error,omitempty"`
}

// EventStatus represents the status of an event
type EventStatus string

const (
	EventStatusPending    EventStatus = "pending"
	EventStatusProcessing EventStatus = "processing"
	EventStatusDelivered  EventStatus = "delivered"
	EventStatusFailed     EventStatus = "failed"
	EventStatusRetrying   EventStatus = "retrying"
)

// EventPayload is the payload sent to the webhook
type EventPayload struct {
	Event     EventData              `json:"event"`
	CreatedAt string                 `json:"created_at"`
	ID        string                 `json:"id"`
	Trigger   TriggerPayload         `json:"trigger"`
	Table     TableInfo              `json:"table"`
}

// EventData contains the event data in the payload
type EventData struct {
	SessionVariables map[string]string      `json:"session_variables,omitempty"`
	Op               string                 `json:"op"`
	Data             DataPayload            `json:"data"`
}

// DataPayload contains old and new data
type DataPayload struct {
	Old map[string]interface{} `json:"old,omitempty"`
	New map[string]interface{} `json:"new,omitempty"`
}

// TriggerPayload contains trigger information in the payload
type TriggerPayload struct {
	Name string `json:"name"`
}

// NewTriggerManager creates a new trigger manager
func NewTriggerManager(db *database.Connection, cfg *config.EventsConfig) *TriggerManager {
	return &TriggerManager{
		db:       db,
		config:   cfg,
		triggers: make(map[string]*EventTrigger),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		workerPool: make(chan struct{}, cfg.HTTPPoolSize),
		eventQueue: make(chan *Event, 10000),
		stopChan:   make(chan struct{}),
	}
}

// Start starts the event trigger manager
func (tm *TriggerManager) Start(ctx context.Context) {
	// Start event processor
	go tm.processEvents(ctx)

	// Start event poller
	go tm.pollEvents(ctx)
}

// Stop stops the event trigger manager
func (tm *TriggerManager) Stop() {
	close(tm.stopChan)
}

// RegisterTrigger registers a new event trigger
func (tm *TriggerManager) RegisterTrigger(trigger *EventTrigger) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.triggers[trigger.Name] = trigger

	// Create database trigger
	if err := tm.createDatabaseTrigger(trigger); err != nil {
		return err
	}

	return nil
}

// UnregisterTrigger removes an event trigger
func (tm *TriggerManager) UnregisterTrigger(name string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	trigger, ok := tm.triggers[name]
	if !ok {
		return fmt.Errorf("trigger not found: %s", name)
	}

	// Drop database trigger
	if err := tm.dropDatabaseTrigger(trigger); err != nil {
		return err
	}

	delete(tm.triggers, name)
	return nil
}

// createDatabaseTrigger creates a PostgreSQL trigger for event capture
func (tm *TriggerManager) createDatabaseTrigger(trigger *EventTrigger) error {
	ctx := context.Background()

	// Create events table if not exists
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS graphpost_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			trigger_name TEXT NOT NULL,
			table_schema TEXT NOT NULL,
			table_name TEXT NOT NULL,
			operation TEXT NOT NULL,
			old_data JSONB,
			new_data JSONB,
			status TEXT DEFAULT 'pending',
			retry_count INTEGER DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			delivered_at TIMESTAMPTZ,
			last_error TEXT,
			delivery_attempts JSONB DEFAULT '[]'::jsonb
		);

		CREATE INDEX IF NOT EXISTS idx_graphpost_events_status ON graphpost_events(status);
		CREATE INDEX IF NOT EXISTS idx_graphpost_events_created_at ON graphpost_events(created_at);
		CREATE INDEX IF NOT EXISTS idx_graphpost_events_trigger ON graphpost_events(trigger_name);
	`

	if _, err := tm.db.Exec(ctx, createTableSQL); err != nil {
		return fmt.Errorf("failed to create events table: %w", err)
	}

	// Create trigger function
	funcName := fmt.Sprintf("graphpost_event_%s", trigger.Name)
	triggerName := fmt.Sprintf("graphpost_trigger_%s", trigger.Name)

	var operations []string
	for _, op := range trigger.Operations {
		operations = append(operations, op)
	}

	createFuncSQL := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s()
		RETURNS trigger AS $$
		BEGIN
			INSERT INTO graphpost_events (trigger_name, table_schema, table_name, operation, old_data, new_data)
			VALUES (
				'%s',
				TG_TABLE_SCHEMA,
				TG_TABLE_NAME,
				TG_OP,
				CASE WHEN TG_OP IN ('UPDATE', 'DELETE') THEN to_jsonb(OLD) ELSE NULL END,
				CASE WHEN TG_OP IN ('INSERT', 'UPDATE') THEN to_jsonb(NEW) ELSE NULL END
			);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`, funcName, trigger.Name)

	if _, err := tm.db.Exec(ctx, createFuncSQL); err != nil {
		return fmt.Errorf("failed to create trigger function: %w", err)
	}

	// Create trigger
	createTriggerSQL := fmt.Sprintf(`
		DROP TRIGGER IF EXISTS %s ON %s.%s;
		CREATE TRIGGER %s
		AFTER %s ON %s.%s
		FOR EACH ROW
		EXECUTE FUNCTION %s();
	`, triggerName, trigger.Schema, trigger.Table,
		triggerName, joinn(operations, " OR "), trigger.Schema, trigger.Table,
		funcName)

	if _, err := tm.db.Exec(ctx, createTriggerSQL); err != nil {
		return fmt.Errorf("failed to create trigger: %w", err)
	}

	return nil
}

// dropDatabaseTrigger drops a PostgreSQL trigger
func (tm *TriggerManager) dropDatabaseTrigger(trigger *EventTrigger) error {
	ctx := context.Background()

	triggerName := fmt.Sprintf("graphpost_trigger_%s", trigger.Name)
	funcName := fmt.Sprintf("graphpost_event_%s", trigger.Name)

	dropSQL := fmt.Sprintf(`
		DROP TRIGGER IF EXISTS %s ON %s.%s;
		DROP FUNCTION IF EXISTS %s();
	`, triggerName, trigger.Schema, trigger.Table, funcName)

	_, err := tm.db.Exec(ctx, dropSQL)
	return err
}

// pollEvents polls for pending events
func (tm *TriggerManager) pollEvents(ctx context.Context) {
	ticker := time.NewTicker(tm.config.FetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tm.stopChan:
			return
		case <-ticker.C:
			tm.fetchPendingEvents(ctx)
		}
	}
}

// fetchPendingEvents fetches and queues pending events
func (tm *TriggerManager) fetchPendingEvents(ctx context.Context) {
	query := `
		SELECT id, trigger_name, table_schema, table_name, operation, old_data, new_data, retry_count, created_at
		FROM graphpost_events
		WHERE status IN ('pending', 'retrying')
		ORDER BY created_at
		LIMIT 100
		FOR UPDATE SKIP LOCKED
	`

	rows, err := tm.db.Query(ctx, query)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var event Event
		var tableSchema, tableName string
		var oldData, newData []byte

		if err := rows.Scan(
			&event.ID,
			&event.TriggerName,
			&tableSchema,
			&tableName,
			&event.Operation,
			&oldData,
			&newData,
			&event.RetryCount,
			&event.CreatedAt,
		); err != nil {
			continue
		}

		event.Table = TableInfo{Schema: tableSchema, Name: tableName}

		if len(oldData) > 0 {
			json.Unmarshal(oldData, &event.OldData)
		}
		if len(newData) > 0 {
			json.Unmarshal(newData, &event.NewData)
		}

		// Mark as processing
		tm.db.Exec(ctx, "UPDATE graphpost_events SET status = 'processing' WHERE id = $1", event.ID)

		// Queue for processing
		select {
		case tm.eventQueue <- &event:
		default:
			// Queue full, will retry later
			tm.db.Exec(ctx, "UPDATE graphpost_events SET status = 'pending' WHERE id = $1", event.ID)
		}
	}
}

// processEvents processes queued events
func (tm *TriggerManager) processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tm.stopChan:
			return
		case event := <-tm.eventQueue:
			// Acquire worker from pool
			tm.workerPool <- struct{}{}
			go func(e *Event) {
				defer func() { <-tm.workerPool }()
				tm.deliverEvent(ctx, e)
			}(event)
		}
	}
}

// deliverEvent delivers an event to the webhook
func (tm *TriggerManager) deliverEvent(ctx context.Context, event *Event) {
	tm.mu.RLock()
	trigger, ok := tm.triggers[event.TriggerName]
	tm.mu.RUnlock()

	if !ok {
		tm.markEventFailed(ctx, event, "trigger not found")
		return
	}

	// Build payload
	payload := EventPayload{
		Event: EventData{
			Op: event.Operation,
			Data: DataPayload{
				Old: event.OldData,
				New: event.NewData,
			},
		},
		CreatedAt: event.CreatedAt.Format(time.RFC3339),
		ID:        event.ID,
		Trigger: TriggerPayload{
			Name: trigger.Name,
		},
		Table: event.Table,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		tm.markEventFailed(ctx, event, err.Error())
		return
	}

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", trigger.WebhookURL, bytes.NewReader(payloadBytes))
	if err != nil {
		tm.markEventFailed(ctx, event, err.Error())
		return
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range trigger.Headers {
		req.Header.Set(key, value)
	}

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		tm.handleDeliveryFailure(ctx, event, trigger, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		tm.markEventDelivered(ctx, event)
	} else {
		tm.handleDeliveryFailure(ctx, event, trigger, fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
}

// markEventDelivered marks an event as delivered
func (tm *TriggerManager) markEventDelivered(ctx context.Context, event *Event) {
	query := `
		UPDATE graphpost_events
		SET status = 'delivered', delivered_at = NOW()
		WHERE id = $1
	`
	tm.db.Exec(ctx, query, event.ID)
}

// markEventFailed marks an event as permanently failed
func (tm *TriggerManager) markEventFailed(ctx context.Context, event *Event, errMsg string) {
	query := `
		UPDATE graphpost_events
		SET status = 'failed', last_error = $1
		WHERE id = $2
	`
	tm.db.Exec(ctx, query, errMsg, event.ID)
}

// handleDeliveryFailure handles a failed delivery attempt
func (tm *TriggerManager) handleDeliveryFailure(ctx context.Context, event *Event, trigger *EventTrigger, errMsg string) {
	maxRetries := tm.config.RetryLimit
	if trigger.RetryConfig != nil {
		maxRetries = trigger.RetryConfig.NumRetries
	}

	if event.RetryCount >= maxRetries {
		tm.markEventFailed(ctx, event, errMsg)
		return
	}

	// Schedule retry
	query := `
		UPDATE graphpost_events
		SET status = 'retrying', retry_count = retry_count + 1, last_error = $1
		WHERE id = $2
	`
	tm.db.Exec(ctx, query, errMsg, event.ID)
}

// InvokeEventTrigger manually invokes an event trigger
func (tm *TriggerManager) InvokeEventTrigger(ctx context.Context, name string, payload map[string]interface{}) error {
	tm.mu.RLock()
	trigger, ok := tm.triggers[name]
	tm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("trigger not found: %s", name)
	}

	if !trigger.EnableManual && !tm.config.EnableManualTrigger {
		return fmt.Errorf("manual trigger invocation not enabled for: %s", name)
	}

	event := &Event{
		ID:          generateUUID(),
		TriggerName: name,
		Table:       TableInfo{Schema: trigger.Schema, Name: trigger.Table},
		Operation:   "MANUAL",
		NewData:     payload,
		CreatedAt:   time.Now(),
		Status:      EventStatusPending,
	}

	tm.eventQueue <- event
	return nil
}

// GetEventLogs retrieves event logs
func (tm *TriggerManager) GetEventLogs(ctx context.Context, triggerName string, limit, offset int) ([]*Event, error) {
	query := `
		SELECT id, trigger_name, table_schema, table_name, operation, old_data, new_data,
			   status, retry_count, created_at, delivered_at, last_error
		FROM graphpost_events
		WHERE trigger_name = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := tm.db.Query(ctx, query, triggerName, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var event Event
		var tableSchema, tableName string
		var oldData, newData []byte
		var lastError *string
		var deliveredAt *time.Time

		if err := rows.Scan(
			&event.ID,
			&event.TriggerName,
			&tableSchema,
			&tableName,
			&event.Operation,
			&oldData,
			&newData,
			&event.Status,
			&event.RetryCount,
			&event.CreatedAt,
			&deliveredAt,
			&lastError,
		); err != nil {
			continue
		}

		event.Table = TableInfo{Schema: tableSchema, Name: tableName}
		if len(oldData) > 0 {
			json.Unmarshal(oldData, &event.OldData)
		}
		if len(newData) > 0 {
			json.Unmarshal(newData, &event.NewData)
		}

		events = append(events, &event)
	}

	return events, nil
}

// RedeliverEvent attempts to redeliver a failed event
func (tm *TriggerManager) RedeliverEvent(ctx context.Context, eventID string) error {
	query := `
		UPDATE graphpost_events
		SET status = 'pending', retry_count = 0
		WHERE id = $1 AND status = 'failed'
	`

	result, err := tm.db.Exec(ctx, query, eventID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("event not found or not in failed state: %s", eventID)
	}

	return nil
}

// Helper functions

func joinn(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

func generateUUID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
