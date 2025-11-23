# GraphPost Client

A Next.js GraphQL client for GraphPost with client-side DuckDB-WASM analytics and configurable logging for query performance analysis.

## Features

- **Apollo GraphQL Client**: Full-featured GraphQL client with caching, subscriptions, and error handling
- **DuckDB-WASM Analytics**: Client-side OLAP queries for local data analysis
- **Configurable Logging**: Performance tracking with slow query detection and optimization suggestions
- **React Hooks**: Custom hooks for easy integration with React components
- **TypeScript**: Full type safety and IntelliSense support

## Installation

```bash
cd client
npm install
```

## Quick Start

### 1. Configure Environment

Create a `.env.local` file:

```bash
NEXT_PUBLIC_GRAPHPOST_ENDPOINT=http://localhost:8080/v1/graphql
NEXT_PUBLIC_GRAPHPOST_ADMIN_SECRET=your-admin-secret
```

### 2. Wrap Your App with Provider

```tsx
import { GraphPostProvider } from '@/lib/provider';

const config = {
  endpoint: process.env.NEXT_PUBLIC_GRAPHPOST_ENDPOINT,
  adminSecret: process.env.NEXT_PUBLIC_GRAPHPOST_ADMIN_SECRET,
  loggerConfig: {
    level: 'debug',
    queryLogging: true,
    slowQueryThreshold: 1000, // 1 second
  },
};

export default function App({ children }) {
  return (
    <GraphPostProvider config={config} autoInitAnalytics>
      {children}
    </GraphPostProvider>
  );
}
```

### 3. Use GraphQL Queries

```tsx
import { gql, useQuery } from '@apollo/client';
import { useGraphPostQuery } from '@/hooks';

const GET_USERS = gql`
  query GetUsers {
    users {
      id
      name
      email
    }
  }
`;

function UserList() {
  const { data, loading, error } = useGraphPostQuery(GET_USERS, {
    enableLocalCache: true,
    localCacheTTL: 300,
    cacheKey: 'users',
  });

  if (loading) return <div>Loading...</div>;
  if (error) return <div>Error: {error.message}</div>;

  return (
    <ul>
      {data.users.map(user => (
        <li key={user.id}>{user.name}</li>
      ))}
    </ul>
  );
}
```

## Configurable Logging

The logging system helps identify slow queries for database optimization.

### Configuration Options

```typescript
interface LoggerConfig {
  // Minimum log level: 'debug' | 'info' | 'warn' | 'error' | 'silent'
  level: LogLevel;

  // Enable query logging
  queryLogging: boolean;

  // Threshold in ms above which queries are considered slow
  slowQueryThreshold: number;

  // Maximum log entries to retain in memory
  maxLogEntries: number;

  // Enable console output
  consoleOutput: boolean;

  // Custom log handler for external logging systems
  customHandler?: (entry: LogEntry) => void;

  // Enable performance metrics collection
  collectMetrics: boolean;
}
```

### Using the Logger

```tsx
import { useLogger } from '@/lib/provider';

function PerformanceMonitor() {
  const logger = useLogger();

  // Get slow query report
  const report = logger.getSlowQueryReport();

  // Export logs for analysis
  const handleExport = () => {
    const jsonData = logger.exportLogs();
    // Download or send to analytics service
  };

  return (
    <div>
      <h2>Slow Queries</h2>
      {report.map(query => (
        <div key={query.operationName}>
          <strong>{query.operationName}</strong>
          <p>Average: {query.averageDuration}ms</p>
          <p>P95: {query.p95Duration}ms</p>
          <ul>
            {query.suggestions.map((s, i) => (
              <li key={i}>{s}</li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}
```

### Slow Query Report

The logger automatically tracks query performance and provides optimization suggestions:

```typescript
interface SlowQueryReport {
  operationName: string;
  averageDuration: number;
  maxDuration: number;
  minDuration: number;
  count: number;
  p95Duration: number;
  suggestions: string[];
}
```

**Example suggestions:**
- "Consider adding database indexes for frequently queried columns"
- "Enable query caching for frequently accessed data"
- "Consider using DuckDB analytics for aggregate queries"
- "Check if the query can be paginated to reduce data volume"

## DuckDB-WASM Analytics

Client-side analytics engine for high-performance local data analysis.

### Initialize Analytics

```tsx
import { useAnalytics } from '@/lib/provider';

function AnalyticsComponent() {
  const { analytics, ready, initialize } = useAnalytics();

  if (!ready) {
    return <button onClick={initialize}>Initialize Analytics</button>;
  }

  // Use analytics...
}
```

### Load and Query Data

```tsx
const { analytics } = useAnalytics();

// Load data from GraphQL result
await analytics.loadFromJSON('orders', ordersData);

// Run SQL queries
const result = await analytics.query(`
  SELECT
    status,
    COUNT(*) as count,
    SUM(total) as revenue
  FROM orders
  GROUP BY status
`);

console.log(result.data);
// [{ status: 'completed', count: 150, revenue: 45000 }, ...]
```

### Aggregate Queries

```tsx
const result = await analytics.aggregate({
  table: 'orders',
  aggregates: [
    { function: 'COUNT', alias: 'order_count' },
    { function: 'SUM', column: 'total', alias: 'total_revenue' },
    { function: 'AVG', column: 'total', alias: 'avg_order_value' },
  ],
  groupBy: ['status', 'region'],
  where: "created_at > '2024-01-01'",
  orderBy: [{ column: 'total_revenue', direction: 'DESC' }],
  limit: 10,
});
```

### Performance Tracking

DuckDB-WASM automatically tracks query performance:

```tsx
const report = await analytics.getPerformanceReport();
// Returns: query_name, avg_duration, max_duration, total_calls, cache_hit_rate
```

## Custom Hooks

### useGraphPostQuery

Enhanced query hook with local caching:

```tsx
const { data, localData, isFromLocalCache, loading, error } = useGraphPostQuery(
  GET_DATA,
  {
    enableLocalCache: true,
    localCacheTTL: 300, // seconds
    cacheKey: 'my-data',
  }
);
```

### useLocalAggregate

Run aggregations on local DuckDB data:

```tsx
const { data, loading, error, duration, refetch } = useLocalAggregate(
  'orders',
  [
    { function: 'SUM', column: 'amount', alias: 'total' },
    { function: 'COUNT', alias: 'count' },
  ],
  {
    groupBy: ['category'],
    where: "status = 'completed'",
  }
);
```

### useLocalData

Load and manage local data:

```tsx
const { ready, rowCount, query, insert } = useLocalData('products', initialData);

// Run queries when ready
if (ready) {
  const result = await query('SELECT * FROM products WHERE price > 100');
}
```

## Project Structure

```
client/
├── src/
│   ├── app/
│   │   ├── layout.tsx      # Root layout
│   │   └── page.tsx        # Main page
│   ├── components/
│   │   ├── Dashboard.tsx   # Main dashboard
│   │   ├── QueryExplorer.tsx
│   │   ├── LogViewer.tsx
│   │   ├── AnalyticsPanel.tsx
│   │   └── PerformanceReport.tsx
│   ├── hooks/
│   │   ├── index.ts
│   │   └── useGraphPostQuery.ts
│   ├── lib/
│   │   ├── index.ts
│   │   ├── graphql-client.ts
│   │   ├── duckdb-analytics.ts
│   │   ├── logger.ts
│   │   └── provider.tsx
│   ├── styles/
│   │   └── globals.css
│   └── types/
├── package.json
├── tsconfig.json
├── next.config.js
├── tailwind.config.js
└── README.md
```

## Development

```bash
# Start development server
npm run dev

# Build for production
npm run build

# Start production server
npm start

# Type check
npm run type-check

# Lint
npm run lint
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NEXT_PUBLIC_GRAPHPOST_ENDPOINT` | GraphQL endpoint URL | `http://localhost:8080/v1/graphql` |
| `NEXT_PUBLIC_GRAPHPOST_ADMIN_SECRET` | Admin secret for authentication | - |

## Browser Support

DuckDB-WASM requires:
- Chrome 80+
- Firefox 79+
- Safari 15+
- Edge 80+

WebAssembly and SharedArrayBuffer support is required for optimal performance.

## License

MIT License
