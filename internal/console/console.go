package console

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

// Console serves the admin console
type Console struct {
	templates *template.Template
	staticFS  fs.FS
	config    *ConsoleConfig
}

// ConsoleConfig holds console configuration
type ConsoleConfig struct {
	GraphQLEndpoint     string
	AdminSecret         string
	EnableTelemetry     bool
	ConsoleType         string
	ServerVersion       string
}

// NewConsole creates a new console instance
func NewConsole(cfg *ConsoleConfig) (*Console, error) {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
	}

	return &Console{
		staticFS: staticFS,
		config:   cfg,
	}, nil
}

// Handler returns the HTTP handler for the console
func (c *Console) Handler() http.Handler {
	mux := http.NewServeMux()

	// Serve static files
	mux.Handle("/console/static/", http.StripPrefix("/console/static/", http.FileServer(http.FS(c.staticFS))))

	// Main console page
	mux.HandleFunc("/console", c.handleConsole)
	mux.HandleFunc("/console/", c.handleConsole)

	// API Explorer
	mux.HandleFunc("/console/api-explorer", c.handleAPIExplorer)
	mux.HandleFunc("/console/api/api-explorer", c.handleAPIExplorer)

	// Data tab
	mux.HandleFunc("/console/data", c.handleData)
	mux.HandleFunc("/console/data/", c.handleData)

	// Actions tab
	mux.HandleFunc("/console/actions", c.handleActions)
	mux.HandleFunc("/console/actions/", c.handleActions)

	// Remote Schemas
	mux.HandleFunc("/console/remote-schemas", c.handleRemoteSchemas)
	mux.HandleFunc("/console/remote-schemas/", c.handleRemoteSchemas)

	// Events
	mux.HandleFunc("/console/events", c.handleEvents)
	mux.HandleFunc("/console/events/", c.handleEvents)

	// Settings
	mux.HandleFunc("/console/settings", c.handleSettings)
	mux.HandleFunc("/console/settings/", c.handleSettings)

	return mux
}

// handleConsole serves the main console page
func (c *Console) handleConsole(w http.ResponseWriter, r *http.Request) {
	html := c.getConsoleHTML()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handleAPIExplorer serves the API Explorer (GraphiQL)
func (c *Console) handleAPIExplorer(w http.ResponseWriter, r *http.Request) {
	c.handleConsole(w, r)
}

// handleData serves the Data tab
func (c *Console) handleData(w http.ResponseWriter, r *http.Request) {
	c.handleConsole(w, r)
}

// handleActions serves the Actions tab
func (c *Console) handleActions(w http.ResponseWriter, r *http.Request) {
	c.handleConsole(w, r)
}

// handleRemoteSchemas serves the Remote Schemas tab
func (c *Console) handleRemoteSchemas(w http.ResponseWriter, r *http.Request) {
	c.handleConsole(w, r)
}

// handleEvents serves the Events tab
func (c *Console) handleEvents(w http.ResponseWriter, r *http.Request) {
	c.handleConsole(w, r)
}

// handleSettings serves the Settings tab
func (c *Console) handleSettings(w http.ResponseWriter, r *http.Request) {
	c.handleConsole(w, r)
}

// getConsoleHTML returns the console HTML
func (c *Console) getConsoleHTML() string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GraphPost Console</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            background-color: #1a1a2e;
            color: #eee;
            min-height: 100vh;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            padding: 1rem 2rem;
            display: flex;
            align-items: center;
            justify-content: space-between;
            box-shadow: 0 2px 10px rgba(0,0,0,0.3);
        }
        .logo {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            font-size: 1.5rem;
            font-weight: bold;
        }
        .logo-icon {
            width: 32px;
            height: 32px;
            background: white;
            border-radius: 8px;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #667eea;
            font-weight: bold;
        }
        nav {
            display: flex;
            gap: 1rem;
        }
        nav a {
            color: rgba(255,255,255,0.8);
            text-decoration: none;
            padding: 0.5rem 1rem;
            border-radius: 4px;
            transition: all 0.2s;
        }
        nav a:hover, nav a.active {
            background: rgba(255,255,255,0.2);
            color: white;
        }
        .container {
            display: flex;
            height: calc(100vh - 60px);
        }
        .sidebar {
            width: 250px;
            background: #16213e;
            padding: 1rem;
            overflow-y: auto;
        }
        .sidebar h3 {
            color: #888;
            font-size: 0.75rem;
            text-transform: uppercase;
            margin-bottom: 0.5rem;
            padding: 0.5rem;
        }
        .sidebar ul {
            list-style: none;
        }
        .sidebar li {
            padding: 0.5rem;
            cursor: pointer;
            border-radius: 4px;
            margin-bottom: 2px;
        }
        .sidebar li:hover {
            background: rgba(255,255,255,0.1);
        }
        .sidebar li.active {
            background: #667eea;
        }
        .main {
            flex: 1;
            padding: 2rem;
            overflow-y: auto;
        }
        .card {
            background: #16213e;
            border-radius: 8px;
            padding: 1.5rem;
            margin-bottom: 1rem;
            box-shadow: 0 2px 10px rgba(0,0,0,0.2);
        }
        .card h2 {
            margin-bottom: 1rem;
            color: #667eea;
        }
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }
        .stat-card {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            padding: 1.5rem;
            border-radius: 8px;
            text-align: center;
        }
        .stat-card h3 {
            font-size: 2rem;
            margin-bottom: 0.5rem;
        }
        .stat-card p {
            opacity: 0.8;
        }
        .graphiql-container {
            background: #1a1a2e;
            border-radius: 8px;
            height: 600px;
            overflow: hidden;
        }
        .graphiql-container iframe {
            width: 100%%;
            height: 100%%;
            border: none;
        }
        .table-list {
            display: grid;
            gap: 0.5rem;
        }
        .table-item {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 1rem;
            background: rgba(255,255,255,0.05);
            border-radius: 4px;
            cursor: pointer;
        }
        .table-item:hover {
            background: rgba(255,255,255,0.1);
        }
        .table-icon {
            width: 32px;
            height: 32px;
            background: #667eea;
            border-radius: 4px;
            display: flex;
            align-items: center;
            justify-content: center;
            margin-right: 1rem;
        }
        .table-info {
            flex: 1;
        }
        .table-name {
            font-weight: 500;
        }
        .table-schema {
            font-size: 0.875rem;
            color: #888;
        }
        .btn {
            padding: 0.5rem 1rem;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 0.875rem;
            transition: all 0.2s;
        }
        .btn-primary {
            background: #667eea;
            color: white;
        }
        .btn-primary:hover {
            background: #5a6fd6;
        }
        .btn-secondary {
            background: rgba(255,255,255,0.1);
            color: white;
        }
        .btn-secondary:hover {
            background: rgba(255,255,255,0.2);
        }
        .tab-content {
            display: none;
        }
        .tab-content.active {
            display: block;
        }
    </style>
</head>
<body>
    <header class="header">
        <div class="logo">
            <div class="logo-icon">GP</div>
            <span>GraphPost Console</span>
        </div>
        <nav>
            <a href="#api" class="active" onclick="showTab('api')">API Explorer</a>
            <a href="#data" onclick="showTab('data')">Data</a>
            <a href="#actions" onclick="showTab('actions')">Actions</a>
            <a href="#events" onclick="showTab('events')">Events</a>
            <a href="#remote" onclick="showTab('remote')">Remote Schemas</a>
            <a href="#settings" onclick="showTab('settings')">Settings</a>
        </nav>
    </header>

    <div class="container">
        <aside class="sidebar">
            <h3>Database</h3>
            <ul id="table-list">
                <li>Loading tables...</li>
            </ul>
        </aside>

        <main class="main">
            <!-- API Explorer Tab -->
            <div id="tab-api" class="tab-content active">
                <div class="stats">
                    <div class="stat-card">
                        <h3 id="table-count">-</h3>
                        <p>Tables</p>
                    </div>
                    <div class="stat-card">
                        <h3 id="view-count">-</h3>
                        <p>Views</p>
                    </div>
                    <div class="stat-card">
                        <h3 id="relationship-count">-</h3>
                        <p>Relationships</p>
                    </div>
                    <div class="stat-card">
                        <h3 id="permission-count">-</h3>
                        <p>Permissions</p>
                    </div>
                </div>

                <div class="card">
                    <h2>GraphQL API Explorer</h2>
                    <div class="graphiql-container">
                        <iframe src="/" title="GraphQL Playground"></iframe>
                    </div>
                </div>
            </div>

            <!-- Data Tab -->
            <div id="tab-data" class="tab-content">
                <div class="card">
                    <h2>Tables</h2>
                    <p style="margin-bottom: 1rem; color: #888;">Manage your database tables and relationships</p>
                    <div class="table-list" id="data-table-list">
                        Loading...
                    </div>
                </div>
            </div>

            <!-- Actions Tab -->
            <div id="tab-actions" class="tab-content">
                <div class="card">
                    <h2>Actions</h2>
                    <p style="color: #888;">Create custom business logic with Actions</p>
                    <br>
                    <button class="btn btn-primary">Create Action</button>
                </div>
            </div>

            <!-- Events Tab -->
            <div id="tab-events" class="tab-content">
                <div class="card">
                    <h2>Event Triggers</h2>
                    <p style="color: #888;">Capture events on database tables and send webhooks</p>
                    <br>
                    <button class="btn btn-primary">Create Event Trigger</button>
                </div>
            </div>

            <!-- Remote Schemas Tab -->
            <div id="tab-remote" class="tab-content">
                <div class="card">
                    <h2>Remote Schemas</h2>
                    <p style="color: #888;">Add remote GraphQL schemas to your API</p>
                    <br>
                    <button class="btn btn-primary">Add Remote Schema</button>
                </div>
            </div>

            <!-- Settings Tab -->
            <div id="tab-settings" class="tab-content">
                <div class="card">
                    <h2>Settings</h2>
                    <p style="color: #888; margin-bottom: 1rem;">Configure your GraphPost instance</p>

                    <h3 style="margin: 1rem 0 0.5rem;">Server Info</h3>
                    <p>Version: %s</p>
                    <p>GraphQL Endpoint: %s</p>

                    <h3 style="margin: 1rem 0 0.5rem;">Authentication</h3>
                    <p>Admin Secret: %s</p>
                </div>
            </div>
        </main>
    </div>

    <script>
        function showTab(tabName) {
            // Hide all tabs
            document.querySelectorAll('.tab-content').forEach(tab => {
                tab.classList.remove('active');
            });
            document.querySelectorAll('nav a').forEach(link => {
                link.classList.remove('active');
            });

            // Show selected tab
            document.getElementById('tab-' + tabName).classList.add('active');
            document.querySelector('nav a[href="#' + tabName + '"]').classList.add('active');
        }

        // Fetch schema info
        async function loadSchemaInfo() {
            try {
                const response = await fetch('/v1/graphql', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({
                        query: '{ __schema { types { name kind } } }'
                    })
                });

                const data = await response.json();
                if (data.data && data.data.__schema) {
                    const types = data.data.__schema.types;
                    const tables = types.filter(t =>
                        t.kind === 'OBJECT' &&
                        !t.name.startsWith('__') &&
                        !['Query', 'Mutation', 'Subscription'].includes(t.name) &&
                        !t.name.includes('_aggregate') &&
                        !t.name.includes('_mutation_response')
                    );

                    document.getElementById('table-count').textContent = tables.length;

                    // Update sidebar
                    const tableList = document.getElementById('table-list');
                    tableList.innerHTML = tables.map(t =>
                        '<li onclick="selectTable(\'' + t.name + '\')">' + t.name + '</li>'
                    ).join('');

                    // Update data tab
                    const dataTableList = document.getElementById('data-table-list');
                    dataTableList.innerHTML = tables.map(t =>
                        '<div class="table-item" onclick="selectTable(\'' + t.name + '\')">' +
                        '<div class="table-icon">T</div>' +
                        '<div class="table-info">' +
                        '<div class="table-name">' + t.name + '</div>' +
                        '<div class="table-schema">public</div>' +
                        '</div>' +
                        '<button class="btn btn-secondary">Browse</button>' +
                        '</div>'
                    ).join('');
                }
            } catch (error) {
                console.error('Failed to load schema:', error);
            }
        }

        function selectTable(tableName) {
            console.log('Selected table:', tableName);
            // Could open table browser here
        }

        // Load on page load
        loadSchemaInfo();
    </script>
</body>
</html>
`, c.config.ServerVersion, c.config.GraphQLEndpoint,
		func() string {
			if c.config.AdminSecret != "" {
				return "Configured"
			}
			return "Not Set"
		}())
}

// StaticContent returns embedded static content
func StaticContent() fs.FS {
	sub, _ := fs.Sub(staticFiles, "static")
	return sub
}
