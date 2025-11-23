/**
 * GraphPost Client-Side DuckDB Analytics
 *
 * Uses DuckDB-WASM for high-performance client-side data analysis.
 * Enables local caching and aggregation of GraphQL query results.
 */

import { getLogger } from './logger';

// DuckDB types (dynamic import)
type DuckDBInstance = {
  connect(): Promise<DuckDBConnection>;
  open(config: unknown): Promise<void>;
  close(): Promise<void>;
};

type DuckDBConnection = {
  query<T = unknown>(sql: string): Promise<{ toArray(): T[] }>;
  prepare(sql: string): Promise<DuckDBStatement>;
  close(): Promise<void>;
};

type DuckDBStatement = {
  query<T = unknown>(...params: unknown[]): Promise<{ toArray(): T[] }>;
  close(): Promise<void>;
};

export interface DuckDBAnalyticsConfig {
  /** Enable DuckDB analytics */
  enabled: boolean;

  /** Maximum memory for DuckDB (in bytes) */
  maxMemory?: number;

  /** Enable query logging */
  logging?: boolean;

  /** Tables to create on initialization */
  tables?: TableDefinition[];
}

export interface TableDefinition {
  name: string;
  columns: ColumnDefinition[];
  primaryKey?: string[];
}

export interface ColumnDefinition {
  name: string;
  type: 'INTEGER' | 'BIGINT' | 'DOUBLE' | 'VARCHAR' | 'BOOLEAN' | 'TIMESTAMP' | 'JSON';
  nullable?: boolean;
}

export interface AggregateQuery {
  table: string;
  aggregates: AggregateFunction[];
  groupBy?: string[];
  where?: string;
  orderBy?: { column: string; direction: 'ASC' | 'DESC' }[];
  limit?: number;
}

export interface AggregateFunction {
  function: 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'COUNT_DISTINCT';
  column?: string;
  alias: string;
}

export interface QueryResult<T = Record<string, unknown>> {
  data: T[];
  duration: number;
  rowCount: number;
}

const DEFAULT_CONFIG: DuckDBAnalyticsConfig = {
  enabled: true,
  logging: true,
  maxMemory: 256 * 1024 * 1024, // 256MB
};

class DuckDBAnalytics {
  private config: DuckDBAnalyticsConfig;
  private db: DuckDBInstance | null = null;
  private conn: DuckDBConnection | null = null;
  private initialized = false;
  private initializing = false;
  private logger = getLogger();

  constructor(config: Partial<DuckDBAnalyticsConfig> = {}) {
    this.config = { ...DEFAULT_CONFIG, ...config };
  }

  /**
   * Initialize DuckDB-WASM
   */
  async initialize(): Promise<boolean> {
    if (this.initialized) return true;
    if (this.initializing) {
      // Wait for initialization to complete
      while (this.initializing) {
        await new Promise(resolve => setTimeout(resolve, 100));
      }
      return this.initialized;
    }

    if (typeof window === 'undefined') {
      this.logger.warn('duckdb', 'DuckDB-WASM is only available in browser environment');
      return false;
    }

    this.initializing = true;
    const startTime = performance.now();

    try {
      this.logger.info('duckdb', 'Initializing DuckDB-WASM...');

      // Dynamic import of DuckDB-WASM
      const duckdb = await import('@duckdb/duckdb-wasm');
      const JSDELIVR_BUNDLES = duckdb.getJsDelivrBundles();

      // Select best bundle for the browser
      const bundle = await duckdb.selectBundle(JSDELIVR_BUNDLES);

      // Instantiate worker
      const worker_url = URL.createObjectURL(
        new Blob([`importScripts("${bundle.mainWorker}");`], {
          type: 'text/javascript',
        })
      );

      const worker = new Worker(worker_url);
      const logger = new duckdb.ConsoleLogger();

      // Instantiate DuckDB
      this.db = await (duckdb as unknown as { AsyncDuckDB: new (logger: unknown, worker: Worker) => Promise<DuckDBInstance> }).AsyncDuckDB.create(logger, worker) as unknown as DuckDBInstance;

      await this.db.open({
        path: ':memory:',
        query: {
          castBigIntToDouble: true,
        },
      });

      // Create connection
      this.conn = await this.db.connect();

      // Create predefined tables
      if (this.config.tables) {
        for (const table of this.config.tables) {
          await this.createTable(table);
        }
      }

      // Create default analytics tables
      await this.createDefaultTables();

      this.initialized = true;
      const duration = Math.round(performance.now() - startTime);

      this.logger.info('duckdb', `DuckDB-WASM initialized (${duration}ms)`);

      return true;
    } catch (error) {
      this.logger.error('duckdb', 'Failed to initialize DuckDB-WASM', {
        error: (error as Error).message,
      });
      return false;
    } finally {
      this.initializing = false;
    }
  }

  /**
   * Create default analytics tables
   */
  private async createDefaultTables(): Promise<void> {
    if (!this.conn) return;

    // Query cache table
    await this.conn.query(`
      CREATE TABLE IF NOT EXISTS _query_cache (
        query_hash VARCHAR PRIMARY KEY,
        query_name VARCHAR,
        result_json VARCHAR,
        cached_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        ttl_seconds INTEGER DEFAULT 300
      )
    `);

    // Query metrics table
    await this.conn.query(`
      CREATE TABLE IF NOT EXISTS _query_metrics (
        id INTEGER PRIMARY KEY,
        query_name VARCHAR,
        duration_ms INTEGER,
        from_cache BOOLEAN,
        executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
      )
    `);

    this.logger.debug('duckdb', 'Created default analytics tables');
  }

  /**
   * Create a table
   */
  async createTable(definition: TableDefinition): Promise<void> {
    if (!this.conn) {
      throw new Error('DuckDB not initialized');
    }

    const columns = definition.columns
      .map(col => {
        const nullable = col.nullable === false ? ' NOT NULL' : '';
        return `${col.name} ${col.type}${nullable}`;
      })
      .join(', ');

    const primaryKey = definition.primaryKey
      ? `, PRIMARY KEY (${definition.primaryKey.join(', ')})`
      : '';

    const sql = `CREATE TABLE IF NOT EXISTS ${definition.name} (${columns}${primaryKey})`;

    await this.conn.query(sql);
    this.logger.debug('duckdb', `Created table: ${definition.name}`);
  }

  /**
   * Execute a raw SQL query
   */
  async query<T = Record<string, unknown>>(sql: string): Promise<QueryResult<T>> {
    if (!this.conn) {
      throw new Error('DuckDB not initialized');
    }

    const startTime = performance.now();

    try {
      const result = await this.conn.query<T>(sql);
      const data = result.toArray();
      const duration = Math.round(performance.now() - startTime);

      if (this.config.logging) {
        this.logger.debug('duckdb', `Query executed (${duration}ms)`, {
          sql: sql.substring(0, 100),
          rowCount: data.length,
        });
      }

      return {
        data,
        duration,
        rowCount: data.length,
      };
    } catch (error) {
      this.logger.error('duckdb', 'Query failed', {
        sql: sql.substring(0, 100),
        error: (error as Error).message,
      });
      throw error;
    }
  }

  /**
   * Execute an aggregate query
   */
  async aggregate<T = Record<string, unknown>>(
    query: AggregateQuery
  ): Promise<QueryResult<T>> {
    const selectClauses: string[] = [];

    // Add group by columns
    if (query.groupBy) {
      selectClauses.push(...query.groupBy);
    }

    // Add aggregate functions
    for (const agg of query.aggregates) {
      let func: string;
      switch (agg.function) {
        case 'COUNT':
          func = agg.column ? `COUNT(${agg.column})` : 'COUNT(*)';
          break;
        case 'COUNT_DISTINCT':
          func = `COUNT(DISTINCT ${agg.column})`;
          break;
        case 'SUM':
          func = `SUM(${agg.column})`;
          break;
        case 'AVG':
          func = `AVG(${agg.column})`;
          break;
        case 'MIN':
          func = `MIN(${agg.column})`;
          break;
        case 'MAX':
          func = `MAX(${agg.column})`;
          break;
        default:
          throw new Error(`Unknown aggregate function: ${agg.function}`);
      }
      selectClauses.push(`${func} AS ${agg.alias}`);
    }

    let sql = `SELECT ${selectClauses.join(', ')} FROM ${query.table}`;

    if (query.where) {
      sql += ` WHERE ${query.where}`;
    }

    if (query.groupBy && query.groupBy.length > 0) {
      sql += ` GROUP BY ${query.groupBy.join(', ')}`;
    }

    if (query.orderBy && query.orderBy.length > 0) {
      const orderClauses = query.orderBy.map(o => `${o.column} ${o.direction}`);
      sql += ` ORDER BY ${orderClauses.join(', ')}`;
    }

    if (query.limit) {
      sql += ` LIMIT ${query.limit}`;
    }

    return this.query<T>(sql);
  }

  /**
   * Insert data into a table
   */
  async insert(table: string, data: Record<string, unknown>[]): Promise<number> {
    if (!this.conn || data.length === 0) {
      return 0;
    }

    const columns = Object.keys(data[0]);
    const placeholders = columns.map(() => '?').join(', ');
    const sql = `INSERT INTO ${table} (${columns.join(', ')}) VALUES (${placeholders})`;

    const stmt = await this.conn.prepare(sql);
    let inserted = 0;

    try {
      for (const row of data) {
        const values = columns.map(col => row[col]);
        await stmt.query(...values);
        inserted++;
      }
    } finally {
      await stmt.close();
    }

    this.logger.debug('duckdb', `Inserted ${inserted} rows into ${table}`);
    return inserted;
  }

  /**
   * Cache GraphQL query results
   */
  async cacheQueryResult(
    queryName: string,
    result: unknown,
    ttlSeconds: number = 300
  ): Promise<void> {
    if (!this.conn) return;

    const queryHash = this.hashString(queryName + JSON.stringify(result));

    await this.query(`
      INSERT OR REPLACE INTO _query_cache (query_hash, query_name, result_json, ttl_seconds)
      VALUES ('${queryHash}', '${queryName}', '${JSON.stringify(result).replace(/'/g, "''")}', ${ttlSeconds})
    `);
  }

  /**
   * Get cached query result
   */
  async getCachedResult<T>(queryName: string): Promise<T | null> {
    if (!this.conn) return null;

    const queryHash = this.hashString(queryName);

    const result = await this.query<{ result_json: string }>(`
      SELECT result_json FROM _query_cache
      WHERE query_hash = '${queryHash}'
        AND cached_at > CURRENT_TIMESTAMP - INTERVAL (ttl_seconds) SECOND
    `);

    if (result.data.length > 0) {
      return JSON.parse(result.data[0].result_json) as T;
    }

    return null;
  }

  /**
   * Record query metrics
   */
  async recordQueryMetrics(
    queryName: string,
    durationMs: number,
    fromCache: boolean
  ): Promise<void> {
    if (!this.conn) return;

    await this.query(`
      INSERT INTO _query_metrics (query_name, duration_ms, from_cache)
      VALUES ('${queryName}', ${durationMs}, ${fromCache})
    `);
  }

  /**
   * Get query performance report
   */
  async getPerformanceReport(): Promise<QueryResult<{
    query_name: string;
    avg_duration: number;
    max_duration: number;
    min_duration: number;
    total_calls: number;
    cache_hit_rate: number;
  }>> {
    if (!this.conn) {
      return { data: [], duration: 0, rowCount: 0 };
    }

    return this.query(`
      SELECT
        query_name,
        ROUND(AVG(duration_ms), 2) as avg_duration,
        MAX(duration_ms) as max_duration,
        MIN(duration_ms) as min_duration,
        COUNT(*) as total_calls,
        ROUND(SUM(CASE WHEN from_cache THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) as cache_hit_rate
      FROM _query_metrics
      GROUP BY query_name
      ORDER BY avg_duration DESC
    `);
  }

  /**
   * Load data from JSON for analysis
   */
  async loadFromJSON(table: string, jsonData: unknown[]): Promise<number> {
    if (!this.conn || jsonData.length === 0) {
      return 0;
    }

    // Create table from first object structure
    const firstRow = jsonData[0] as Record<string, unknown>;
    const columns: ColumnDefinition[] = Object.entries(firstRow).map(([name, value]) => ({
      name,
      type: this.inferType(value),
    }));

    await this.createTable({ name: table, columns });
    return this.insert(table, jsonData as Record<string, unknown>[]);
  }

  /**
   * Infer DuckDB type from JavaScript value
   */
  private inferType(value: unknown): ColumnDefinition['type'] {
    if (typeof value === 'number') {
      return Number.isInteger(value) ? 'INTEGER' : 'DOUBLE';
    }
    if (typeof value === 'boolean') {
      return 'BOOLEAN';
    }
    if (value instanceof Date) {
      return 'TIMESTAMP';
    }
    if (typeof value === 'object') {
      return 'JSON';
    }
    return 'VARCHAR';
  }

  /**
   * Simple string hash function
   */
  private hashString(str: string): string {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
      const char = str.charCodeAt(i);
      hash = (hash << 5) - hash + char;
      hash = hash & hash;
    }
    return Math.abs(hash).toString(16);
  }

  /**
   * Clear all cached data
   */
  async clearCache(): Promise<void> {
    if (!this.conn) return;

    await this.query('DELETE FROM _query_cache');
    this.logger.info('duckdb', 'Cleared query cache');
  }

  /**
   * Get DuckDB status
   */
  getStatus(): { initialized: boolean; config: DuckDBAnalyticsConfig } {
    return {
      initialized: this.initialized,
      config: this.config,
    };
  }

  /**
   * Close DuckDB connection
   */
  async close(): Promise<void> {
    if (this.conn) {
      await this.conn.close();
      this.conn = null;
    }
    if (this.db) {
      await this.db.close();
      this.db = null;
    }
    this.initialized = false;
    this.logger.info('duckdb', 'DuckDB connection closed');
  }
}

// Singleton instance
let analyticsInstance: DuckDBAnalytics | null = null;

/**
 * Get or create the DuckDB analytics instance
 */
export function getAnalytics(config?: Partial<DuckDBAnalyticsConfig>): DuckDBAnalytics {
  if (!analyticsInstance) {
    analyticsInstance = new DuckDBAnalytics(config);
  }
  return analyticsInstance;
}

/**
 * Create a new analytics instance (for isolated analysis)
 */
export function createAnalytics(config?: Partial<DuckDBAnalyticsConfig>): DuckDBAnalytics {
  return new DuckDBAnalytics(config);
}

export default DuckDBAnalytics;
