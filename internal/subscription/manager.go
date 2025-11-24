package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/graphpost/graphpost/internal/database"
	"github.com/graphpost/graphpost/internal/resolver"
	"github.com/jackc/pgx/v5/pgconn"
)

// Manager handles GraphQL subscriptions using PostgreSQL LISTEN/NOTIFY
type Manager struct {
	db           *database.Connection
	resolver     *resolver.Resolver
	schema       *database.Schema
	listeners    map[string]*database.Listener
	subscribers  map[string]map[string]*Subscriber
	mu           sync.RWMutex
	pollInterval time.Duration
}

// Subscriber represents a subscription client
type Subscriber struct {
	ID          string
	TableName   string
	Query       string
	Variables   map[string]interface{}
	Channel     chan *SubscriptionEvent
	Done        chan struct{}
	CreatedAt   time.Time
}

// SubscriptionEvent represents an event sent to subscribers
type SubscriptionEvent struct {
	Type    EventType              `json:"type"`
	Table   string                 `json:"table"`
	Data    interface{}            `json:"data"`
	OldData interface{}            `json:"old_data,omitempty"`
	Error   error                  `json:"error,omitempty"`
}

// EventType represents the type of database event
type EventType string

const (
	EventTypeInsert EventType = "INSERT"
	EventTypeUpdate EventType = "UPDATE"
	EventTypeDelete EventType = "DELETE"
	EventTypeError  EventType = "ERROR"
)

// NotificationPayload represents the payload from PostgreSQL NOTIFY
type NotificationPayload struct {
	Table     string                 `json:"table"`
	Operation string                 `json:"operation"`
	Data      map[string]interface{} `json:"data"`
	OldData   map[string]interface{} `json:"old_data,omitempty"`
}

// NewManager creates a new subscription manager
func NewManager(db *database.Connection, res *resolver.Resolver, schema *database.Schema) *Manager {
	return &Manager{
		db:           db,
		resolver:     res,
		schema:       schema,
		listeners:    make(map[string]*database.Listener),
		subscribers:  make(map[string]map[string]*Subscriber),
		pollInterval: 10 * time.Second,
	}
}

// Start starts the subscription manager
func (m *Manager) Start(ctx context.Context) error {
	// Set up listeners for each table
	for tableName := range m.schema.Tables {
		if err := m.setupTableListener(ctx, tableName); err != nil {
			return fmt.Errorf("failed to setup listener for table %s: %w", tableName, err)
		}
	}

	return nil
}

// Stop stops the subscription manager
func (m *Manager) Stop(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all listeners
	for _, listener := range m.listeners {
		listener.Stop(ctx)
	}

	// Close all subscriber channels
	for _, tableSubscribers := range m.subscribers {
		for _, sub := range tableSubscribers {
			close(sub.Done)
		}
	}
}

// Subscribe adds a new subscriber
func (m *Manager) Subscribe(sub *Subscriber) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subscribers[sub.TableName] == nil {
		m.subscribers[sub.TableName] = make(map[string]*Subscriber)
	}

	m.subscribers[sub.TableName][sub.ID] = sub
	return nil
}

// Unsubscribe removes a subscriber
func (m *Manager) Unsubscribe(tableName, subscriberID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if tableSubscribers, ok := m.subscribers[tableName]; ok {
		if sub, ok := tableSubscribers[subscriberID]; ok {
			close(sub.Done)
			delete(tableSubscribers, subscriberID)
		}
	}
}

// setupTableListener sets up a PostgreSQL listener for a table using pgx
func (m *Manager) setupTableListener(ctx context.Context, tableName string) error {
	channelName := fmt.Sprintf("graphpost_%s", tableName)

	// Create callback that will handle notifications
	callback := func(notification *pgconn.Notification) {
		var payload NotificationPayload
		if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
			return
		}
		m.broadcastEvent(tableName, &payload)
	}

	// Create listener using pgx
	listener, err := database.NewListener(m.db.Config(), channelName, callback)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	// Start the listener
	if err := listener.Start(ctx); err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	m.listeners[tableName] = listener
	return nil
}

// broadcastEvent broadcasts an event to all subscribers of a table
func (m *Manager) broadcastEvent(tableName string, payload *NotificationPayload) {
	m.mu.RLock()
	subscribers := m.subscribers[tableName]
	m.mu.RUnlock()

	if len(subscribers) == 0 {
		return
	}

	eventType := EventType(payload.Operation)
	event := &SubscriptionEvent{
		Type:    eventType,
		Table:   tableName,
		Data:    payload.Data,
		OldData: payload.OldData,
	}

	for _, sub := range subscribers {
		select {
		case sub.Channel <- event:
		case <-sub.Done:
			// Subscriber is closed
		default:
			// Channel is full, skip
		}
	}
}

// SetupTriggers creates database triggers for real-time subscriptions
func (m *Manager) SetupTriggers(ctx context.Context) error {
	// Create the notification function
	functionSQL := `
		CREATE OR REPLACE FUNCTION graphpost_notify_trigger()
		RETURNS trigger AS $$
		DECLARE
			channel TEXT;
			payload JSONB;
			old_data JSONB;
			new_data JSONB;
		BEGIN
			channel := 'graphpost_' || TG_TABLE_NAME;

			IF TG_OP = 'DELETE' THEN
				old_data := to_jsonb(OLD);
				payload := jsonb_build_object(
					'table', TG_TABLE_NAME,
					'operation', TG_OP,
					'old_data', old_data
				);
			ELSIF TG_OP = 'UPDATE' THEN
				old_data := to_jsonb(OLD);
				new_data := to_jsonb(NEW);
				payload := jsonb_build_object(
					'table', TG_TABLE_NAME,
					'operation', TG_OP,
					'data', new_data,
					'old_data', old_data
				);
			ELSIF TG_OP = 'INSERT' THEN
				new_data := to_jsonb(NEW);
				payload := jsonb_build_object(
					'table', TG_TABLE_NAME,
					'operation', TG_OP,
					'data', new_data
				);
			END IF;

			PERFORM pg_notify(channel, payload::text);

			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`

	_, err := m.db.Exec(ctx, functionSQL)
	if err != nil {
		return fmt.Errorf("failed to create notification function: %w", err)
	}

	// Create triggers for each table
	for tableName := range m.schema.Tables {
		triggerSQL := fmt.Sprintf(`
			DROP TRIGGER IF EXISTS graphpost_trigger ON %s;
			CREATE TRIGGER graphpost_trigger
			AFTER INSERT OR UPDATE OR DELETE ON %s
			FOR EACH ROW
			EXECUTE FUNCTION graphpost_notify_trigger();
		`, tableName, tableName)

		if _, err := m.db.Exec(ctx, triggerSQL); err != nil {
			return fmt.Errorf("failed to create trigger for table %s: %w", tableName, err)
		}
	}

	return nil
}

// LiveQueryManager handles live queries (polling-based subscriptions)
type LiveQueryManager struct {
	resolver     *resolver.Resolver
	subscriptions map[string]*LiveQuery
	mu           sync.RWMutex
	pollInterval time.Duration
}

// LiveQuery represents a live query subscription
type LiveQuery struct {
	ID           string
	TableName    string
	Params       resolver.QueryParams
	Channel      chan []map[string]interface{}
	Done         chan struct{}
	lastResult   []map[string]interface{}
	lastHash     string
}

// NewLiveQueryManager creates a new live query manager
func NewLiveQueryManager(res *resolver.Resolver, pollInterval time.Duration) *LiveQueryManager {
	return &LiveQueryManager{
		resolver:      res,
		subscriptions: make(map[string]*LiveQuery),
		pollInterval:  pollInterval,
	}
}

// Start starts the live query manager
func (lm *LiveQueryManager) Start(ctx context.Context) {
	ticker := time.NewTicker(lm.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lm.pollAllQueries(ctx)
		}
	}
}

// Subscribe adds a new live query
func (lm *LiveQueryManager) Subscribe(lq *LiveQuery) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.subscriptions[lq.ID] = lq
}

// Unsubscribe removes a live query
func (lm *LiveQueryManager) Unsubscribe(id string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lq, ok := lm.subscriptions[id]; ok {
		close(lq.Done)
		delete(lm.subscriptions, id)
	}
}

// pollAllQueries polls all active live queries
func (lm *LiveQueryManager) pollAllQueries(ctx context.Context) {
	lm.mu.RLock()
	queries := make([]*LiveQuery, 0, len(lm.subscriptions))
	for _, lq := range lm.subscriptions {
		queries = append(queries, lq)
	}
	lm.mu.RUnlock()

	for _, lq := range queries {
		go lm.pollQuery(ctx, lq)
	}
}

// pollQuery polls a single live query
func (lm *LiveQueryManager) pollQuery(ctx context.Context, lq *LiveQuery) {
	results, err := lm.resolver.ResolveQuery(ctx, lq.Params)
	if err != nil {
		return
	}

	// Check if results changed
	newHash := hashResults(results)
	if newHash == lq.lastHash {
		return
	}

	lq.lastResult = results
	lq.lastHash = newHash

	select {
	case lq.Channel <- results:
	case <-lq.Done:
	default:
	}
}

// hashResults creates a hash of the results for change detection
func hashResults(results []map[string]interface{}) string {
	data, _ := json.Marshal(results)
	return string(data)
}
