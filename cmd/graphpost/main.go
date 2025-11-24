package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/graphpost/graphpost/internal/config"
	"github.com/graphpost/graphpost/internal/engine"
)

var (
	version   = "1.0.0"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	// Parse command line flags
	var (
		configFile     string
		host           string
		port           int
		dbURL          string
		adminSecret    string
		jwtSecret      string
		enableConsole  bool
		enablePlayground bool
		showVersion    bool
		showHelp       bool
	)

	flag.StringVar(&configFile, "config", "", "Path to configuration file")
	flag.StringVar(&host, "host", "0.0.0.0", "Server host")
	flag.IntVar(&port, "port", 8080, "Server port")
	flag.StringVar(&dbURL, "database-url", "", "PostgreSQL database URL")
	flag.StringVar(&adminSecret, "admin-secret", "", "Admin secret for authentication")
	flag.StringVar(&jwtSecret, "jwt-secret", "", "JWT secret for authentication")
	flag.BoolVar(&enableConsole, "enable-console", true, "Enable admin console")
	flag.BoolVar(&enablePlayground, "enable-playground", true, "Enable GraphQL Playground")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&showHelp, "help", false, "Show help")

	flag.Parse()

	if showHelp {
		printHelp()
		return
	}

	if showVersion {
		fmt.Printf("GraphPost %s\n", version)
		fmt.Printf("Build time: %s\n", buildTime)
		fmt.Printf("Git commit: %s\n", gitCommit)
		return
	}

	// Load configuration
	cfg := loadConfig(configFile, host, port, dbURL, adminSecret, jwtSecret, enableConsole, enablePlayground)

	// Print banner
	printBanner()

	// Create and initialize engine
	eng := engine.NewEngine(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("🔌 Connecting to database...")
	if err := eng.Initialize(ctx); err != nil {
		fmt.Printf("❌ Failed to initialize engine: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Database connected successfully")

	// Print schema info
	dbSchema := eng.GetDBSchema()
	fmt.Printf("📋 Found %d tables, %d views\n", len(dbSchema.Tables), len(dbSchema.Views))

	// Start engine
	if err := eng.Start(ctx); err != nil {
		fmt.Printf("❌ Failed to start engine: %v\n", err)
		os.Exit(1)
	}

	// Create and start server
	server := engine.NewServer(eng)

	// Create shutdown manager with graceful shutdown configuration
	shutdownConfig := engine.ShutdownConfig{
		Timeout:      30 * time.Second,
		DrainTimeout: 15 * time.Second,
		ForceAfter:   45 * time.Second,
	}
	shutdownMgr := engine.NewShutdownManager(shutdownConfig)

	// Register components for graceful shutdown (in reverse order of startup)
	// Server should stop accepting new connections first
	shutdownMgr.Register(server)
	// Engine handles subscriptions, triggers, and database
	shutdownMgr.Register(eng)

	// Set shutdown hooks for logging
	shutdownMgr.OnShutdownStart(func() {
		fmt.Println("\n🛑 Graceful shutdown initiated...")
	})
	shutdownMgr.OnDrainStart(func() {
		fmt.Println("⏳ Draining active connections...")
	})
	shutdownMgr.OnDrainEnd(func() {
		fmt.Println("✅ Connections drained")
	})
	shutdownMgr.OnShutdownEnd(func(err error) {
		if err != nil {
			fmt.Printf("⚠️  Shutdown completed with errors: %v\n", err)
		} else {
			fmt.Println("✅ Graceful shutdown completed successfully")
		}
		fmt.Println("👋 Goodbye!")
	})

	// Handle shutdown signals
	go func() {
		sig := shutdownMgr.WaitForSignal()
		fmt.Printf("Received signal: %s\n", sig)
		cancel()

		if err := shutdownMgr.Shutdown(context.Background()); err != nil {
			fmt.Printf("Shutdown error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()

	// Start server
	if err := server.Start(); err != nil {
		fmt.Printf("❌ Server error: %v\n", err)
		os.Exit(1)
	}
}

func loadConfig(configFile, host string, port int, dbURL, adminSecret, jwtSecret string, enableConsole, enablePlayground bool) *config.Config {
	var cfg *config.Config

	if configFile != "" {
		var err error
		cfg, err = config.LoadFromFile(configFile)
		if err != nil {
			fmt.Printf("Warning: Failed to load config file: %v\n", err)
			cfg = config.DefaultConfig()
		}
	} else {
		cfg = config.LoadFromEnv()
	}

	// Override with command line flags
	if host != "" {
		cfg.Server.Host = host
	}
	if port != 0 {
		cfg.Server.Port = port
	}
	if dbURL != "" {
		cfg.Database = parseDatabaseURL(dbURL)
	}
	if adminSecret != "" {
		cfg.Auth.AdminSecret = adminSecret
		cfg.Auth.Enabled = true
	}
	if jwtSecret != "" {
		cfg.Auth.JWTSecret = jwtSecret
	}
	cfg.Console.Enabled = enableConsole
	cfg.Server.EnablePlayground = enablePlayground

	// Load from environment variables (highest priority)
	if envDBURL := os.Getenv("GRAPHPOST_DATABASE_URL"); envDBURL != "" {
		cfg.Database = parseDatabaseURL(envDBURL)
	}
	if envHost := os.Getenv("GRAPHPOST_HOST"); envHost != "" {
		cfg.Server.Host = envHost
	}
	if envPort := os.Getenv("GRAPHPOST_PORT"); envPort != "" {
		var p int
		fmt.Sscanf(envPort, "%d", &p)
		if p > 0 {
			cfg.Server.Port = p
		}
	}
	if envAdminSecret := os.Getenv("GRAPHPOST_ADMIN_SECRET"); envAdminSecret != "" {
		cfg.Auth.AdminSecret = envAdminSecret
		cfg.Auth.Enabled = true
	}
	if envJWTSecret := os.Getenv("GRAPHPOST_JWT_SECRET"); envJWTSecret != "" {
		cfg.Auth.JWTSecret = envJWTSecret
	}

	return cfg
}

func parseDatabaseURL(url string) config.DatabaseConfig {
	cfg := config.DatabaseConfig{
		Host:            "localhost",
		Port:            5432,
		User:            "postgres",
		Password:        "",
		Database:        "postgres",
		SSLMode:         "disable",
		MaxOpenConns:    50,
		MaxIdleConns:    10,
		Schema:          "public",
	}

	// Parse URL format: postgres://user:password@host:port/database?sslmode=disable
	// Simple parsing - use net/url for production
	url = url[len("postgres://"):]

	// Extract user:password@host:port/database
	atIdx := indexOf(url, "@")
	if atIdx > 0 {
		userPass := url[:atIdx]
		url = url[atIdx+1:]

		colonIdx := indexOf(userPass, ":")
		if colonIdx > 0 {
			cfg.User = userPass[:colonIdx]
			cfg.Password = userPass[colonIdx+1:]
		} else {
			cfg.User = userPass
		}
	}

	// Extract host:port/database
	slashIdx := indexOf(url, "/")
	if slashIdx > 0 {
		hostPort := url[:slashIdx]
		url = url[slashIdx+1:]

		colonIdx := indexOf(hostPort, ":")
		if colonIdx > 0 {
			cfg.Host = hostPort[:colonIdx]
			fmt.Sscanf(hostPort[colonIdx+1:], "%d", &cfg.Port)
		} else {
			cfg.Host = hostPort
		}
	}

	// Extract database?params
	questionIdx := indexOf(url, "?")
	if questionIdx > 0 {
		cfg.Database = url[:questionIdx]
		// Parse params
		params := url[questionIdx+1:]
		for _, param := range splitString(params, "&") {
			kv := splitString(param, "=")
			if len(kv) == 2 {
				switch kv[0] {
				case "sslmode":
					cfg.SSLMode = kv[1]
				case "schema", "search_path":
					cfg.Schema = kv[1]
				}
			}
		}
	} else {
		cfg.Database = url
	}

	return cfg
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func splitString(s, sep string) []string {
	var result []string
	for {
		idx := indexOf(s, sep)
		if idx < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	return result
}

func printBanner() {
	banner := `
   ____                 _     ____           _
  / ___|_ __ __ _ _ __ | |__ |  _ \ ___  ___| |_
 | |  _| '__/ _' | '_ \| '_ \| |_) / _ \/ __| __|
 | |_| | | | (_| | |_) | | | |  __/ (_) \__ \ |_
  \____|_|  \__,_| .__/|_| |_|_|   \___/|___/\__|
                 |_|
                                          v%s
  GraphQL API for PostgreSQL - Hasura-compatible
  ================================================
`
	fmt.Printf(banner, version)
}

func printHelp() {
	fmt.Println("GraphPost - Instant GraphQL API for PostgreSQL")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  graphpost [flags]")
	fmt.Println("")
	fmt.Println("Flags:")
	fmt.Println("  --config string        Path to configuration file")
	fmt.Println("  --host string          Server host (default \"0.0.0.0\")")
	fmt.Println("  --port int             Server port (default 8080)")
	fmt.Println("  --database-url string  PostgreSQL database URL")
	fmt.Println("  --admin-secret string  Admin secret for authentication")
	fmt.Println("  --jwt-secret string    JWT secret for authentication")
	fmt.Println("  --enable-console       Enable admin console (default true)")
	fmt.Println("  --enable-playground    Enable GraphQL Playground (default true)")
	fmt.Println("  --version              Show version")
	fmt.Println("  --help                 Show help")
	fmt.Println("")
	fmt.Println("Environment Variables:")
	fmt.Println("  GRAPHPOST_DATABASE_URL    PostgreSQL connection URL")
	fmt.Println("  GRAPHPOST_HOST            Server host")
	fmt.Println("  GRAPHPOST_PORT            Server port")
	fmt.Println("  GRAPHPOST_ADMIN_SECRET    Admin secret")
	fmt.Println("  GRAPHPOST_JWT_SECRET      JWT secret")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  # Start with database URL")
	fmt.Println("  graphpost --database-url postgres://user:pass@localhost:5432/mydb")
	fmt.Println("")
	fmt.Println("  # Start with environment variable")
	fmt.Println("  GRAPHPOST_DATABASE_URL=postgres://localhost/mydb graphpost")
	fmt.Println("")
	fmt.Println("  # Start with config file")
	fmt.Println("  graphpost --config /path/to/config.json")
	fmt.Println("")
}
