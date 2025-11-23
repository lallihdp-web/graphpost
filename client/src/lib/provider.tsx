'use client';

/**
 * GraphPost Provider
 *
 * React context provider that combines Apollo Client, DuckDB Analytics, and Logging.
 */

import React, { createContext, useContext, useEffect, useState, useCallback, ReactNode } from 'react';
import { ApolloProvider, ApolloClient, NormalizedCacheObject } from '@apollo/client';
import { createGraphPostClient, GraphPostClientConfig } from './graphql-client';
import { getAnalytics, DuckDBAnalyticsConfig, DuckDBAnalytics } from './duckdb-analytics';
import { getLogger, LoggerConfig, LogEntry, SlowQueryReport } from './logger';

export interface GraphPostContextValue {
  client: ApolloClient<NormalizedCacheObject>;
  analytics: DuckDBAnalytics | null;
  analyticsReady: boolean;
  initializeAnalytics: () => Promise<boolean>;
  logger: {
    getLogs: (filter?: { level?: string; category?: string; since?: Date }) => LogEntry[];
    getSlowQueryReport: () => SlowQueryReport[];
    clearLogs: () => void;
    exportLogs: () => string;
    configure: (config: Partial<LoggerConfig>) => void;
  };
}

const GraphPostContext = createContext<GraphPostContextValue | null>(null);

export interface GraphPostProviderProps {
  children: ReactNode;
  config: GraphPostClientConfig;
  analyticsConfig?: Partial<DuckDBAnalyticsConfig>;
  autoInitAnalytics?: boolean;
}

export function GraphPostProvider({
  children,
  config,
  analyticsConfig,
  autoInitAnalytics = false,
}: GraphPostProviderProps) {
  const [client] = useState(() => createGraphPostClient(config));
  const [analytics] = useState(() => getAnalytics(analyticsConfig));
  const [analyticsReady, setAnalyticsReady] = useState(false);
  const logger = getLogger(config.loggerConfig);

  const initializeAnalytics = useCallback(async (): Promise<boolean> => {
    if (analyticsReady) return true;

    const success = await analytics.initialize();
    setAnalyticsReady(success);
    return success;
  }, [analytics, analyticsReady]);

  useEffect(() => {
    if (autoInitAnalytics) {
      initializeAnalytics();
    }
  }, [autoInitAnalytics, initializeAnalytics]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      analytics.close();
      logger.destroy();
    };
  }, [analytics, logger]);

  const contextValue: GraphPostContextValue = {
    client,
    analytics: analyticsReady ? analytics : null,
    analyticsReady,
    initializeAnalytics,
    logger: {
      getLogs: (filter) => logger.getLogs(filter as Parameters<typeof logger.getLogs>[0]),
      getSlowQueryReport: () => logger.getSlowQueryReport(),
      clearLogs: () => logger.clearLogs(),
      exportLogs: () => logger.exportLogs(),
      configure: (cfg) => logger.configure(cfg),
    },
  };

  return (
    <GraphPostContext.Provider value={contextValue}>
      <ApolloProvider client={client}>{children}</ApolloProvider>
    </GraphPostContext.Provider>
  );
}

/**
 * Hook to access GraphPost context
 */
export function useGraphPost(): GraphPostContextValue {
  const context = useContext(GraphPostContext);
  if (!context) {
    throw new Error('useGraphPost must be used within a GraphPostProvider');
  }
  return context;
}

/**
 * Hook to access DuckDB analytics
 */
export function useAnalytics() {
  const { analytics, analyticsReady, initializeAnalytics } = useGraphPost();

  return {
    analytics,
    ready: analyticsReady,
    initialize: initializeAnalytics,
    query: analytics?.query.bind(analytics),
    aggregate: analytics?.aggregate.bind(analytics),
    loadFromJSON: analytics?.loadFromJSON.bind(analytics),
    getPerformanceReport: analytics?.getPerformanceReport.bind(analytics),
  };
}

/**
 * Hook to access logging functionality
 */
export function useLogger() {
  const { logger } = useGraphPost();
  return logger;
}

export default GraphPostProvider;
