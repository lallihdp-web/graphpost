package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CDCMode represents the CDC operation mode
type CDCMode string

const (
	// CDCModePolling uses pg_stat_user_tables polling
	CDCModePolling CDCMode = "polling"

	// CDCModeRealtime uses PostgreSQL LISTEN/NOTIFY
	CDCModeRealtime CDCMode = "realtime"

	// CDCModeBoth uses both polling and realtime
	CDCModeBoth CDCMode = "both"
)

// CDCNotificationChannel is the PostgreSQL notification channel for CDC events
const CDCNotificationChannel = "graphpost_cdc"

// CDCEvent represents a change data capture event
type CDCEvent struct {
	Table     string                 `json:"table"`
	Operation string                 `json:"operation"` // INSERT, UPDATE, DELETE
	Timestamp time.Time              `json:"timestamp"`
	OldData   map[string]interface{} `json:"old_data,omitempty"`
	NewData   map[string]interface{} `json:"new_data,omitempty"`
}

// CDCListener listens for real-time CDC events via PostgreSQL LISTEN/NOTIFY
type CDCListener struct {
	pool            *pgxpool.Pool
	conn            *pgx.Conn
	config          CDCListenerConfig
	callback        func(event *CDCEvent)
	running         bool
	stop            chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	monitoredTables map[string]bool
	triggersCreated map[string]bool
}

// CDCListenerConfig holds configuration for the CDC listener
type CDCListenerConfig struct {
	// Mode is the CDC operation mode (polling, realtime, both)
	Mode CDCMode

	// PollInterval is the interval for polling mode
	PollInterval time.Duration

	// ReconnectDelay is the delay before attempting to reconnect
	ReconnectDelay time.Duration

	// CreateTriggers automatically creates CDC triggers on monitored tables
	CreateTriggers bool

	// TriggerPrefix is the prefix for created trigger names
	TriggerPrefix string

	// IncludeRowData includes old/new row data in CDC events
	IncludeRowData bool

	// DatabaseURL for establishing the dedicated listen connection
	DatabaseURL string
}

// NewCDCListener creates a new CDC listener
func NewCDCListener(pool *pgxpool.Pool, config CDCListenerConfig) *CDCListener {
	if config.PollInterval == 0 {
		config.PollInterval = 5 * time.Second
	}
	if config.ReconnectDelay == 0 {
		config.ReconnectDelay = 5 * time.Second
	}
	if config.TriggerPrefix == "" {
		config.TriggerPrefix = "graphpost_cdc_"
	}

	return &CDCListener{
		pool:            pool,
		config:          config,
		monitoredTables: make(map[string]bool),
		triggersCreated: make(map[string]bool),
	}
}

// SetCallback sets the callback function for CDC events
func (l *CDCListener) SetCallback(callback func(event *CDCEvent)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.callback = callback
}

// AddTable adds a table to be monitored for changes
func (l *CDCListener) AddTable(tableName string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.monitoredTables[tableName] = true

	// Create trigger if configured and listener is running
	if l.config.CreateTriggers && l.running && !l.triggersCreated[tableName] {
		if err := l.createTriggerForTable(context.Background(), tableName); err != nil {
			return fmt.Errorf("failed to create trigger for table %s: %w", tableName, err)
		}
	}

	return nil
}

// RemoveTable removes a table from monitoring
func (l *CDCListener) RemoveTable(tableName string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.monitoredTables, tableName)
}

// Start starts the CDC listener
func (l *CDCListener) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return nil
	}
	l.running = true
	l.stop = make(chan struct{})
	l.mu.Unlock()

	// Create triggers for monitored tables if configured
	if l.config.CreateTriggers {
		if err := l.createTriggersForAllTables(ctx); err != nil {
			log.Printf("[CDC] Warning: Failed to create some triggers: %v", err)
		}
	}

	// Start the listener based on mode
	switch l.config.Mode {
	case CDCModeRealtime:
		l.wg.Add(1)
		go l.listenLoop()
		log.Printf("[CDC] Real-time listener started (LISTEN/NOTIFY)")

	case CDCModePolling:
		l.wg.Add(1)
		go l.pollLoop()
		log.Printf("[CDC] Polling listener started (interval: %v)", l.config.PollInterval)

	case CDCModeBoth:
		l.wg.Add(2)
		go l.listenLoop()
		go l.pollLoop()
		log.Printf("[CDC] Both listeners started (realtime + polling at %v)", l.config.PollInterval)

	default:
		return fmt.Errorf("unknown CDC mode: %s", l.config.Mode)
	}

	return nil
}

// Stop stops the CDC listener
func (l *CDCListener) Stop() {
	l.mu.Lock()
	if !l.running {
		l.mu.Unlock()
		return
	}
	l.running = false
	close(l.stop)
	l.mu.Unlock()

	// Close the dedicated listen connection
	if l.conn != nil {
		l.conn.Close(context.Background())
	}

	l.wg.Wait()
	log.Printf("[CDC] Listener stopped")
}

// listenLoop handles real-time LISTEN/NOTIFY events
func (l *CDCListener) listenLoop() {
	defer l.wg.Done()

	for {
		select {
		case <-l.stop:
			return
		default:
			if err := l.connectAndListen(); err != nil {
				log.Printf("[CDC] Listen error: %v, reconnecting in %v", err, l.config.ReconnectDelay)
				select {
				case <-l.stop:
					return
				case <-time.After(l.config.ReconnectDelay):
					continue
				}
			}
		}
	}
}

// connectAndListen establishes connection and listens for notifications
func (l *CDCListener) connectAndListen() error {
	ctx := context.Background()

	// Establish dedicated connection for LISTEN
	var err error
	if l.config.DatabaseURL != "" {
		l.conn, err = pgx.Connect(ctx, l.config.DatabaseURL)
	} else {
		// Acquire connection from pool and convert to dedicated connection
		poolConn, err := l.pool.Acquire(ctx)
		if err != nil {
			return fmt.Errorf("failed to acquire connection: %w", err)
		}
		l.conn = poolConn.Hijack()
	}

	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() {
		if l.conn != nil {
			l.conn.Close(ctx)
			l.conn = nil
		}
	}()

	// Subscribe to CDC channel
	_, err = l.conn.Exec(ctx, fmt.Sprintf("LISTEN %s", CDCNotificationChannel))
	if err != nil {
		return fmt.Errorf("LISTEN failed: %w", err)
	}

	log.Printf("[CDC] Listening on channel: %s", CDCNotificationChannel)

	// Wait for notifications
	for {
		select {
		case <-l.stop:
			return nil
		default:
			// Wait for notification with timeout
			notification, err := l.conn.WaitForNotification(ctx)
			if err != nil {
				return fmt.Errorf("notification wait failed: %w", err)
			}

			l.handleNotification(notification)
		}
	}
}

// handleNotification processes a PostgreSQL notification
func (l *CDCListener) handleNotification(notification *pgconn.Notification) {
	if notification.Channel != CDCNotificationChannel {
		return
	}

	var event CDCEvent
	if err := json.Unmarshal([]byte(notification.Payload), &event); err != nil {
		log.Printf("[CDC] Failed to parse notification payload: %v", err)
		return
	}

	// Check if table is monitored
	l.mu.Lock()
	monitored := l.monitoredTables[event.Table]
	callback := l.callback
	l.mu.Unlock()

	if !monitored {
		return
	}

	if callback != nil {
		callback(&event)
	}
}

// pollLoop handles polling-based CDC detection
func (l *CDCListener) pollLoop() {
	defer l.wg.Done()

	// Track previous row counts for change detection
	prevStats := make(map[string]tableStats)

	ticker := time.NewTicker(l.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			l.pollChanges(prevStats)
		}
	}
}

type tableStats struct {
	inserts int64
	updates int64
	deletes int64
}

// pollChanges polls pg_stat_user_tables for changes
func (l *CDCListener) pollChanges(prevStats map[string]tableStats) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	l.mu.Lock()
	tables := make([]string, 0, len(l.monitoredTables))
	for table := range l.monitoredTables {
		tables = append(tables, table)
	}
	callback := l.callback
	l.mu.Unlock()

	if len(tables) == 0 {
		return
	}

	// Query statistics for monitored tables
	rows, err := l.pool.Query(ctx, `
		SELECT relname, n_tup_ins, n_tup_upd, n_tup_del
		FROM pg_stat_user_tables
		WHERE schemaname = 'public' AND relname = ANY($1)
	`, tables)
	if err != nil {
		log.Printf("[CDC] Poll query failed: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		var inserts, updates, deletes int64
		if err := rows.Scan(&tableName, &inserts, &updates, &deletes); err != nil {
			continue
		}

		prev, exists := prevStats[tableName]
		if !exists {
			prevStats[tableName] = tableStats{inserts, updates, deletes}
			continue
		}

		// Check for changes
		hasInserts := inserts > prev.inserts
		hasUpdates := updates > prev.updates
		hasDeletes := deletes > prev.deletes

		// Update stats
		prevStats[tableName] = tableStats{inserts, updates, deletes}

		if callback != nil {
			if hasInserts {
				callback(&CDCEvent{
					Table:     tableName,
					Operation: "INSERT",
					Timestamp: time.Now(),
				})
			}
			if hasUpdates {
				callback(&CDCEvent{
					Table:     tableName,
					Operation: "UPDATE",
					Timestamp: time.Now(),
				})
			}
			if hasDeletes {
				callback(&CDCEvent{
					Table:     tableName,
					Operation: "DELETE",
					Timestamp: time.Now(),
				})
			}
		}
	}
}

// createTriggersForAllTables creates CDC triggers for all monitored tables
func (l *CDCListener) createTriggersForAllTables(ctx context.Context) error {
	var errs []string

	for table := range l.monitoredTables {
		if l.triggersCreated[table] {
			continue
		}
		if err := l.createTriggerForTable(ctx, table); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", table, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("trigger creation errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// createTriggerForTable creates CDC trigger and function for a table
func (l *CDCListener) createTriggerForTable(ctx context.Context, tableName string) error {
	funcName := l.config.TriggerPrefix + tableName + "_fn"
	triggerName := l.config.TriggerPrefix + tableName + "_trigger"

	// Create the notification function
	createFuncSQL := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s()
		RETURNS TRIGGER AS $$
		DECLARE
			payload JSON;
		BEGIN
			payload = json_build_object(
				'table', TG_TABLE_NAME,
				'operation', TG_OP,
				'timestamp', NOW()
			);

			IF TG_OP = 'DELETE' THEN
				%s
			ELSIF TG_OP = 'UPDATE' THEN
				%s
			ELSIF TG_OP = 'INSERT' THEN
				%s
			END IF;

			PERFORM pg_notify('%s', payload::text);

			IF TG_OP = 'DELETE' THEN
				RETURN OLD;
			ELSE
				RETURN NEW;
			END IF;
		END;
		$$ LANGUAGE plpgsql;
	`,
		funcName,
		l.buildRowDataSQL("old_data", "OLD"),
		l.buildRowDataSQL("new_data", "NEW")+" "+l.buildRowDataSQL("old_data", "OLD"),
		l.buildRowDataSQL("new_data", "NEW"),
		CDCNotificationChannel,
	)

	if _, err := l.pool.Exec(ctx, createFuncSQL); err != nil {
		return fmt.Errorf("failed to create function: %w", err)
	}

	// Create the trigger
	createTriggerSQL := fmt.Sprintf(`
		DROP TRIGGER IF EXISTS %s ON %s;
		CREATE TRIGGER %s
		AFTER INSERT OR UPDATE OR DELETE ON %s
		FOR EACH ROW
		EXECUTE FUNCTION %s();
	`, triggerName, tableName, triggerName, tableName, funcName)

	if _, err := l.pool.Exec(ctx, createTriggerSQL); err != nil {
		return fmt.Errorf("failed to create trigger: %w", err)
	}

	l.triggersCreated[tableName] = true
	log.Printf("[CDC] Created trigger for table: %s", tableName)

	return nil
}

// buildRowDataSQL builds the SQL for including row data in the payload
func (l *CDCListener) buildRowDataSQL(key, rowRef string) string {
	if l.config.IncludeRowData {
		return fmt.Sprintf("payload = payload || jsonb_build_object('%s', to_jsonb(%s));", key, rowRef)
	}
	return ""
}

// DropTrigger drops the CDC trigger for a table
func (l *CDCListener) DropTrigger(ctx context.Context, tableName string) error {
	triggerName := l.config.TriggerPrefix + tableName + "_trigger"
	funcName := l.config.TriggerPrefix + tableName + "_fn"

	_, err := l.pool.Exec(ctx, fmt.Sprintf(`
		DROP TRIGGER IF EXISTS %s ON %s;
		DROP FUNCTION IF EXISTS %s();
	`, triggerName, tableName, funcName))

	if err != nil {
		return err
	}

	delete(l.triggersCreated, tableName)
	log.Printf("[CDC] Dropped trigger for table: %s", tableName)

	return nil
}

// DropAllTriggers drops all CDC triggers
func (l *CDCListener) DropAllTriggers(ctx context.Context) error {
	var errs []string

	for table := range l.triggersCreated {
		if err := l.DropTrigger(ctx, table); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", table, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("trigger drop errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// Status returns the current status of the CDC listener
func (l *CDCListener) Status() CDCListenerStatus {
	l.mu.Lock()
	defer l.mu.Unlock()

	tables := make([]string, 0, len(l.monitoredTables))
	for table := range l.monitoredTables {
		tables = append(tables, table)
	}

	triggersCreated := make([]string, 0, len(l.triggersCreated))
	for table := range l.triggersCreated {
		triggersCreated = append(triggersCreated, table)
	}

	return CDCListenerStatus{
		Running:         l.running,
		Mode:            l.config.Mode,
		MonitoredTables: tables,
		TriggersCreated: triggersCreated,
	}
}

// CDCListenerStatus represents the status of the CDC listener
type CDCListenerStatus struct {
	Running         bool
	Mode            CDCMode
	MonitoredTables []string
	TriggersCreated []string
}
