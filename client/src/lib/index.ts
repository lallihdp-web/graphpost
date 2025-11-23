/**
 * GraphPost Client Library
 *
 * Main exports for the GraphPost Next.js client.
 */

// GraphQL Client
export {
  createGraphPostClient,
  executeQuery,
  executeMutation,
  type GraphPostClientConfig,
  type QueryResult,
} from './graphql-client';

// DuckDB Analytics
export {
  getAnalytics,
  createAnalytics,
  type DuckDBAnalyticsConfig,
  type TableDefinition,
  type ColumnDefinition,
  type AggregateQuery,
  type AggregateFunction,
  type QueryResult as AnalyticsQueryResult,
} from './duckdb-analytics';

// Logger
export {
  getLogger,
  createLogger,
  type LogLevel,
  type LogEntry,
  type QueryLogEntry,
  type SlowQueryReport,
  type LoggerConfig,
} from './logger';

// Provider
export {
  GraphPostProvider,
  useGraphPost,
  useAnalytics,
  useLogger,
  type GraphPostProviderProps,
  type GraphPostContextValue,
} from './provider';
