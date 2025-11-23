/**
 * GraphPost Client Logger
 *
 * Configurable logging system for query performance analysis and debugging.
 * Helps identify slow queries for database optimization.
 */

export type LogLevel = 'debug' | 'info' | 'warn' | 'error' | 'silent';

export interface LogEntry {
  timestamp: Date;
  level: LogLevel;
  category: string;
  message: string;
  duration?: number;
  metadata?: Record<string, unknown>;
}

export interface QueryLogEntry extends LogEntry {
  category: 'query';
  operationName?: string;
  operationType: 'query' | 'mutation' | 'subscription';
  variables?: Record<string, unknown>;
  duration: number;
  fromCache: boolean;
  error?: Error;
}

export interface SlowQueryReport {
  operationName: string;
  averageDuration: number;
  maxDuration: number;
  minDuration: number;
  count: number;
  p95Duration: number;
  suggestions: string[];
}

export interface LoggerConfig {
  /** Minimum log level to output */
  level: LogLevel;

  /** Enable query logging */
  queryLogging: boolean;

  /** Threshold in ms above which queries are considered slow */
  slowQueryThreshold: number;

  /** Maximum number of log entries to retain in memory */
  maxLogEntries: number;

  /** Enable console output */
  consoleOutput: boolean;

  /** Custom log handler for external logging systems */
  customHandler?: (entry: LogEntry) => void;

  /** Enable performance metrics collection */
  collectMetrics: boolean;

  /** Interval in ms to aggregate slow query reports */
  reportingInterval: number;
}

const LOG_LEVEL_PRIORITY: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
  silent: 4,
};

const DEFAULT_CONFIG: LoggerConfig = {
  level: 'info',
  queryLogging: true,
  slowQueryThreshold: 1000, // 1 second
  maxLogEntries: 1000,
  consoleOutput: true,
  collectMetrics: true,
  reportingInterval: 60000, // 1 minute
};

class GraphPostLogger {
  private config: LoggerConfig;
  private logs: LogEntry[] = [];
  private queryMetrics: Map<string, number[]> = new Map();
  private reportingTimer?: ReturnType<typeof setInterval>;

  constructor(config: Partial<LoggerConfig> = {}) {
    this.config = { ...DEFAULT_CONFIG, ...config };

    if (this.config.collectMetrics && typeof window !== 'undefined') {
      this.startReporting();
    }
  }

  /**
   * Configure the logger
   */
  configure(config: Partial<LoggerConfig>): void {
    this.config = { ...this.config, ...config };

    if (this.config.collectMetrics && !this.reportingTimer) {
      this.startReporting();
    } else if (!this.config.collectMetrics && this.reportingTimer) {
      this.stopReporting();
    }
  }

  /**
   * Get current configuration
   */
  getConfig(): LoggerConfig {
    return { ...this.config };
  }

  private shouldLog(level: LogLevel): boolean {
    return LOG_LEVEL_PRIORITY[level] >= LOG_LEVEL_PRIORITY[this.config.level];
  }

  private formatEntry(entry: LogEntry): string {
    const timestamp = entry.timestamp.toISOString();
    const duration = entry.duration ? ` [${entry.duration}ms]` : '';
    return `[${timestamp}] [${entry.level.toUpperCase()}] [${entry.category}]${duration} ${entry.message}`;
  }

  private addEntry(entry: LogEntry): void {
    this.logs.push(entry);

    // Trim old entries if exceeding max
    if (this.logs.length > this.config.maxLogEntries) {
      this.logs = this.logs.slice(-this.config.maxLogEntries);
    }

    // Output to console
    if (this.config.consoleOutput && this.shouldLog(entry.level)) {
      const formatted = this.formatEntry(entry);
      switch (entry.level) {
        case 'debug':
          console.debug(formatted, entry.metadata);
          break;
        case 'info':
          console.info(formatted, entry.metadata);
          break;
        case 'warn':
          console.warn(formatted, entry.metadata);
          break;
        case 'error':
          console.error(formatted, entry.metadata);
          break;
      }
    }

    // Call custom handler
    if (this.config.customHandler) {
      this.config.customHandler(entry);
    }
  }

  /**
   * Log a debug message
   */
  debug(category: string, message: string, metadata?: Record<string, unknown>): void {
    this.addEntry({
      timestamp: new Date(),
      level: 'debug',
      category,
      message,
      metadata,
    });
  }

  /**
   * Log an info message
   */
  info(category: string, message: string, metadata?: Record<string, unknown>): void {
    this.addEntry({
      timestamp: new Date(),
      level: 'info',
      category,
      message,
      metadata,
    });
  }

  /**
   * Log a warning message
   */
  warn(category: string, message: string, metadata?: Record<string, unknown>): void {
    this.addEntry({
      timestamp: new Date(),
      level: 'warn',
      category,
      message,
      metadata,
    });
  }

  /**
   * Log an error message
   */
  error(category: string, message: string, metadata?: Record<string, unknown>): void {
    this.addEntry({
      timestamp: new Date(),
      level: 'error',
      category,
      message,
      metadata,
    });
  }

  /**
   * Log a GraphQL query with performance tracking
   */
  logQuery(entry: Omit<QueryLogEntry, 'timestamp' | 'level' | 'category'>): void {
    if (!this.config.queryLogging) return;

    const isSlowQuery = entry.duration > this.config.slowQueryThreshold;
    const level: LogLevel = isSlowQuery ? 'warn' : 'debug';
    const operationName = entry.operationName || 'Anonymous';

    const logEntry: QueryLogEntry = {
      ...entry,
      timestamp: new Date(),
      level,
      category: 'query',
      message: `${entry.operationType.toUpperCase()} ${operationName}${entry.fromCache ? ' (cached)' : ''}${isSlowQuery ? ' [SLOW]' : ''}`,
    };

    this.addEntry(logEntry);

    // Track metrics for reporting
    if (this.config.collectMetrics) {
      const key = `${entry.operationType}:${operationName}`;
      const durations = this.queryMetrics.get(key) || [];
      durations.push(entry.duration);
      this.queryMetrics.set(key, durations);
    }

    // Log slow query warning with suggestions
    if (isSlowQuery) {
      const suggestions = this.generateSlowQuerySuggestions(entry);
      this.warn('performance', `Slow query detected: ${operationName} (${entry.duration}ms)`, {
        operationName,
        duration: entry.duration,
        threshold: this.config.slowQueryThreshold,
        suggestions,
        variables: entry.variables,
      });
    }
  }

  /**
   * Generate optimization suggestions for slow queries
   */
  private generateSlowQuerySuggestions(entry: Omit<QueryLogEntry, 'timestamp' | 'level' | 'category'>): string[] {
    const suggestions: string[] = [];
    const duration = entry.duration;

    if (duration > 5000) {
      suggestions.push('Consider adding database indexes for frequently queried columns');
      suggestions.push('Check if the query can be paginated to reduce data volume');
      suggestions.push('Consider using materialized aggregates for complex calculations');
    }

    if (duration > 2000) {
      suggestions.push('Enable query caching for frequently accessed data');
      suggestions.push('Consider using DuckDB analytics for aggregate queries');
    }

    if (!entry.fromCache) {
      suggestions.push('This query was not cached - consider enabling cache for repeated queries');
    }

    if (entry.variables && Object.keys(entry.variables).length === 0) {
      suggestions.push('Query has no variables - may be fetching too much data');
    }

    return suggestions;
  }

  /**
   * Get slow query report for optimization analysis
   */
  getSlowQueryReport(): SlowQueryReport[] {
    const reports: SlowQueryReport[] = [];

    for (const [key, durations] of this.queryMetrics.entries()) {
      const sorted = [...durations].sort((a, b) => a - b);
      const avgDuration = durations.reduce((a, b) => a + b, 0) / durations.length;

      if (avgDuration > this.config.slowQueryThreshold * 0.5) {
        const [operationType, operationName] = key.split(':');
        const p95Index = Math.floor(sorted.length * 0.95);

        reports.push({
          operationName: `${operationType}:${operationName}`,
          averageDuration: Math.round(avgDuration),
          maxDuration: sorted[sorted.length - 1],
          minDuration: sorted[0],
          count: durations.length,
          p95Duration: sorted[p95Index] || sorted[sorted.length - 1],
          suggestions: this.generateReportSuggestions(avgDuration, sorted),
        });
      }
    }

    return reports.sort((a, b) => b.averageDuration - a.averageDuration);
  }

  private generateReportSuggestions(avgDuration: number, durations: number[]): string[] {
    const suggestions: string[] = [];
    const variance = this.calculateVariance(durations);

    if (avgDuration > this.config.slowQueryThreshold) {
      suggestions.push('Consider database index optimization');
      suggestions.push('Review query complexity and reduce nested selections');
    }

    if (variance > avgDuration * 0.5) {
      suggestions.push('High variance in query times - may indicate lock contention');
    }

    return suggestions;
  }

  private calculateVariance(numbers: number[]): number {
    const mean = numbers.reduce((a, b) => a + b, 0) / numbers.length;
    return numbers.reduce((sum, n) => sum + Math.pow(n - mean, 2), 0) / numbers.length;
  }

  /**
   * Start periodic reporting
   */
  private startReporting(): void {
    if (this.reportingTimer) return;

    this.reportingTimer = setInterval(() => {
      const report = this.getSlowQueryReport();
      if (report.length > 0) {
        this.info('metrics', `Slow query report: ${report.length} queries need attention`, {
          report,
        });
      }
    }, this.config.reportingInterval);
  }

  /**
   * Stop periodic reporting
   */
  private stopReporting(): void {
    if (this.reportingTimer) {
      clearInterval(this.reportingTimer);
      this.reportingTimer = undefined;
    }
  }

  /**
   * Get all log entries
   */
  getLogs(filter?: { level?: LogLevel; category?: string; since?: Date }): LogEntry[] {
    let filtered = [...this.logs];

    if (filter?.level) {
      const minPriority = LOG_LEVEL_PRIORITY[filter.level];
      filtered = filtered.filter(e => LOG_LEVEL_PRIORITY[e.level] >= minPriority);
    }

    if (filter?.category) {
      filtered = filtered.filter(e => e.category === filter.category);
    }

    if (filter?.since) {
      filtered = filtered.filter(e => e.timestamp >= filter.since!);
    }

    return filtered;
  }

  /**
   * Get query logs only
   */
  getQueryLogs(): QueryLogEntry[] {
    return this.logs.filter((e): e is QueryLogEntry => e.category === 'query');
  }

  /**
   * Clear all logs
   */
  clearLogs(): void {
    this.logs = [];
  }

  /**
   * Clear metrics
   */
  clearMetrics(): void {
    this.queryMetrics.clear();
  }

  /**
   * Export logs as JSON
   */
  exportLogs(): string {
    return JSON.stringify({
      exportedAt: new Date().toISOString(),
      config: this.config,
      logs: this.logs,
      slowQueryReport: this.getSlowQueryReport(),
    }, null, 2);
  }

  /**
   * Cleanup resources
   */
  destroy(): void {
    this.stopReporting();
    this.clearLogs();
    this.clearMetrics();
  }
}

// Singleton instance
let loggerInstance: GraphPostLogger | null = null;

/**
 * Get or create the logger instance
 */
export function getLogger(config?: Partial<LoggerConfig>): GraphPostLogger {
  if (!loggerInstance) {
    loggerInstance = new GraphPostLogger(config);
  } else if (config) {
    loggerInstance.configure(config);
  }
  return loggerInstance;
}

/**
 * Create a new logger instance (for testing or isolated logging)
 */
export function createLogger(config?: Partial<LoggerConfig>): GraphPostLogger {
  return new GraphPostLogger(config);
}

export default GraphPostLogger;
