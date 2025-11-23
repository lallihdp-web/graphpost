package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ShutdownManager handles graceful shutdown of all components
type ShutdownManager struct {
	// Components to shut down (in order)
	components []ShutdownComponent

	// Configuration
	timeout         time.Duration
	drainTimeout    time.Duration
	forceAfter      time.Duration

	// State
	shuttingDown    atomic.Bool
	shutdownStarted time.Time
	activeRequests  atomic.Int64
	mu              sync.Mutex

	// Hooks
	onShutdownStart func()
	onShutdownEnd   func(err error)
	onDrainStart    func()
	onDrainEnd      func()

	// Logger
	logger ShutdownLogger
}

// ShutdownComponent represents a component that can be gracefully shut down
type ShutdownComponent interface {
	Name() string
	Shutdown(ctx context.Context) error
}

// ShutdownLogger interface for logging shutdown events
type ShutdownLogger interface {
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// DefaultShutdownLogger is a simple logger that prints to stdout
type DefaultShutdownLogger struct{}

func (l *DefaultShutdownLogger) Info(msg string, args ...interface{}) {
	log.Printf("[SHUTDOWN] INFO: "+msg, args...)
}

func (l *DefaultShutdownLogger) Warn(msg string, args ...interface{}) {
	log.Printf("[SHUTDOWN] WARN: "+msg, args...)
}

func (l *DefaultShutdownLogger) Error(msg string, args ...interface{}) {
	log.Printf("[SHUTDOWN] ERROR: "+msg, args...)
}

// ShutdownConfig holds configuration for the shutdown manager
type ShutdownConfig struct {
	// Timeout is the total timeout for shutdown
	Timeout time.Duration

	// DrainTimeout is how long to wait for in-flight requests to complete
	DrainTimeout time.Duration

	// ForceAfter forces shutdown after this duration even if requests are pending
	ForceAfter time.Duration

	// Logger for shutdown events
	Logger ShutdownLogger
}

// DefaultShutdownConfig returns the default shutdown configuration
func DefaultShutdownConfig() ShutdownConfig {
	return ShutdownConfig{
		Timeout:      30 * time.Second,
		DrainTimeout: 15 * time.Second,
		ForceAfter:   45 * time.Second,
		Logger:       &DefaultShutdownLogger{},
	}
}

// NewShutdownManager creates a new shutdown manager
func NewShutdownManager(config ShutdownConfig) *ShutdownManager {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.DrainTimeout == 0 {
		config.DrainTimeout = 15 * time.Second
	}
	if config.ForceAfter == 0 {
		config.ForceAfter = 45 * time.Second
	}
	if config.Logger == nil {
		config.Logger = &DefaultShutdownLogger{}
	}

	return &ShutdownManager{
		components:   make([]ShutdownComponent, 0),
		timeout:      config.Timeout,
		drainTimeout: config.DrainTimeout,
		forceAfter:   config.ForceAfter,
		logger:       config.Logger,
	}
}

// Register adds a component to be shut down
func (sm *ShutdownManager) Register(component ShutdownComponent) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.components = append(sm.components, component)
	sm.logger.Info("Registered component for shutdown: %s", component.Name())
}

// RegisterFunc registers a shutdown function as a component
func (sm *ShutdownManager) RegisterFunc(name string, fn func(ctx context.Context) error) {
	sm.Register(&funcComponent{name: name, fn: fn})
}

// funcComponent wraps a function as a ShutdownComponent
type funcComponent struct {
	name string
	fn   func(ctx context.Context) error
}

func (c *funcComponent) Name() string {
	return c.name
}

func (c *funcComponent) Shutdown(ctx context.Context) error {
	return c.fn(ctx)
}

// OnShutdownStart sets a hook that is called when shutdown starts
func (sm *ShutdownManager) OnShutdownStart(fn func()) {
	sm.onShutdownStart = fn
}

// OnShutdownEnd sets a hook that is called when shutdown ends
func (sm *ShutdownManager) OnShutdownEnd(fn func(err error)) {
	sm.onShutdownEnd = fn
}

// OnDrainStart sets a hook that is called when draining starts
func (sm *ShutdownManager) OnDrainStart(fn func()) {
	sm.onDrainStart = fn
}

// OnDrainEnd sets a hook that is called when draining ends
func (sm *ShutdownManager) OnDrainEnd(fn func()) {
	sm.onDrainEnd = fn
}

// IncrementRequests increments the active request counter
func (sm *ShutdownManager) IncrementRequests() {
	sm.activeRequests.Add(1)
}

// DecrementRequests decrements the active request counter
func (sm *ShutdownManager) DecrementRequests() {
	sm.activeRequests.Add(-1)
}

// ActiveRequests returns the number of active requests
func (sm *ShutdownManager) ActiveRequests() int64 {
	return sm.activeRequests.Load()
}

// IsShuttingDown returns true if shutdown has been initiated
func (sm *ShutdownManager) IsShuttingDown() bool {
	return sm.shuttingDown.Load()
}

// WaitForSignal blocks until a shutdown signal is received
func (sm *ShutdownManager) WaitForSignal() os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	sig := <-sigChan
	sm.logger.Info("Received signal: %s", sig)
	return sig
}

// Shutdown performs graceful shutdown of all registered components
func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
	// Prevent multiple shutdown calls
	if !sm.shuttingDown.CompareAndSwap(false, true) {
		sm.logger.Warn("Shutdown already in progress")
		return nil
	}

	sm.shutdownStarted = time.Now()
	sm.logger.Info("Starting graceful shutdown (timeout: %v)", sm.timeout)

	// Call shutdown start hook
	if sm.onShutdownStart != nil {
		sm.onShutdownStart()
	}

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, sm.timeout)
	defer cancel()

	// Start force shutdown timer
	forceTimer := time.AfterFunc(sm.forceAfter, func() {
		sm.logger.Error("Force shutdown triggered after %v", sm.forceAfter)
		os.Exit(1)
	})
	defer forceTimer.Stop()

	var shutdownErr error

	// Phase 1: Drain active requests
	if err := sm.drainRequests(ctx); err != nil {
		sm.logger.Warn("Request draining incomplete: %v", err)
	}

	// Phase 2: Shut down components in reverse order
	sm.mu.Lock()
	components := make([]ShutdownComponent, len(sm.components))
	copy(components, sm.components)
	sm.mu.Unlock()

	for i := len(components) - 1; i >= 0; i-- {
		component := components[i]
		sm.logger.Info("Shutting down: %s", component.Name())

		startTime := time.Now()
		if err := component.Shutdown(ctx); err != nil {
			sm.logger.Error("Failed to shutdown %s: %v", component.Name(), err)
			if shutdownErr == nil {
				shutdownErr = fmt.Errorf("%s: %w", component.Name(), err)
			}
		} else {
			sm.logger.Info("Shutdown complete: %s (took %v)", component.Name(), time.Since(startTime))
		}
	}

	duration := time.Since(sm.shutdownStarted)
	sm.logger.Info("Graceful shutdown complete (took %v)", duration)

	// Call shutdown end hook
	if sm.onShutdownEnd != nil {
		sm.onShutdownEnd(shutdownErr)
	}

	return shutdownErr
}

// drainRequests waits for active requests to complete
func (sm *ShutdownManager) drainRequests(ctx context.Context) error {
	if sm.onDrainStart != nil {
		sm.onDrainStart()
	}
	defer func() {
		if sm.onDrainEnd != nil {
			sm.onDrainEnd()
		}
	}()

	sm.logger.Info("Draining active requests (timeout: %v)", sm.drainTimeout)

	drainCtx, cancel := context.WithTimeout(ctx, sm.drainTimeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		active := sm.activeRequests.Load()
		if active == 0 {
			sm.logger.Info("All requests drained")
			return nil
		}

		select {
		case <-drainCtx.Done():
			remaining := sm.activeRequests.Load()
			return fmt.Errorf("drain timeout: %d requests still active", remaining)
		case <-ticker.C:
			sm.logger.Info("Waiting for %d active requests to complete...", active)
		}
	}
}

// Status returns the current shutdown status
func (sm *ShutdownManager) Status() ShutdownStatus {
	return ShutdownStatus{
		ShuttingDown:   sm.shuttingDown.Load(),
		ActiveRequests: sm.activeRequests.Load(),
		StartedAt:      sm.shutdownStarted,
		Timeout:        sm.timeout,
		DrainTimeout:   sm.drainTimeout,
	}
}

// ShutdownStatus represents the current shutdown state
type ShutdownStatus struct {
	ShuttingDown   bool
	ActiveRequests int64
	StartedAt      time.Time
	Timeout        time.Duration
	DrainTimeout   time.Duration
}
