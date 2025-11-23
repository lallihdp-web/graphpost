'use client';

/**
 * Custom hooks for GraphPost queries with analytics integration
 */

import { useEffect, useMemo, useCallback, useState } from 'react';
import {
  useQuery,
  useMutation,
  DocumentNode,
  TypedDocumentNode,
  QueryHookOptions,
  MutationHookOptions,
  OperationVariables,
  QueryResult,
  MutationTuple,
} from '@apollo/client';
import { useGraphPost, useAnalytics } from '@/lib/provider';

/**
 * Enhanced query hook with DuckDB caching and performance tracking
 */
export function useGraphPostQuery<
  TData = unknown,
  TVariables extends OperationVariables = OperationVariables
>(
  query: DocumentNode | TypedDocumentNode<TData, TVariables>,
  options?: QueryHookOptions<TData, TVariables> & {
    /** Enable DuckDB local caching */
    enableLocalCache?: boolean;
    /** Local cache TTL in seconds */
    localCacheTTL?: number;
    /** Cache key for local storage */
    cacheKey?: string;
  }
): QueryResult<TData, TVariables> & {
  localData: TData | null;
  isFromLocalCache: boolean;
} {
  const { analytics, analyticsReady } = useGraphPost();
  const [localData, setLocalData] = useState<TData | null>(null);
  const [isFromLocalCache, setIsFromLocalCache] = useState(false);

  const {
    enableLocalCache = false,
    localCacheTTL = 300,
    cacheKey,
    ...queryOptions
  } = options || {};

  const result = useQuery<TData, TVariables>(query, queryOptions);

  // Try to get from local cache first
  useEffect(() => {
    if (enableLocalCache && analyticsReady && analytics && cacheKey) {
      analytics.getCachedResult<TData>(cacheKey).then((cached) => {
        if (cached) {
          setLocalData(cached);
          setIsFromLocalCache(true);
        }
      });
    }
  }, [enableLocalCache, analyticsReady, analytics, cacheKey]);

  // Cache successful results
  useEffect(() => {
    if (
      enableLocalCache &&
      analyticsReady &&
      analytics &&
      cacheKey &&
      result.data &&
      !result.loading &&
      !result.error
    ) {
      analytics.cacheQueryResult(cacheKey, result.data, localCacheTTL);
      setLocalData(result.data);
      setIsFromLocalCache(false);
    }
  }, [
    enableLocalCache,
    analyticsReady,
    analytics,
    cacheKey,
    localCacheTTL,
    result.data,
    result.loading,
    result.error,
  ]);

  return {
    ...result,
    localData: localData || result.data || null,
    isFromLocalCache,
  };
}

/**
 * Enhanced mutation hook with logging
 */
export function useGraphPostMutation<
  TData = unknown,
  TVariables extends OperationVariables = OperationVariables
>(
  mutation: DocumentNode | TypedDocumentNode<TData, TVariables>,
  options?: MutationHookOptions<TData, TVariables>
): MutationTuple<TData, TVariables> {
  return useMutation<TData, TVariables>(mutation, options);
}

/**
 * Hook to run aggregations on local DuckDB data
 */
export function useLocalAggregate<T = Record<string, unknown>>(
  table: string,
  aggregates: { function: string; column?: string; alias: string }[],
  options?: {
    groupBy?: string[];
    where?: string;
    orderBy?: { column: string; direction: 'ASC' | 'DESC' }[];
    limit?: number;
    enabled?: boolean;
  }
) {
  const { analytics, analyticsReady } = useGraphPost();
  const [data, setData] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [duration, setDuration] = useState<number>(0);

  const { enabled = true, ...queryOptions } = options || {};

  const aggregateKey = useMemo(
    () => JSON.stringify({ table, aggregates, ...queryOptions }),
    [table, aggregates, queryOptions]
  );

  const runAggregate = useCallback(async () => {
    if (!analyticsReady || !analytics || !enabled) return;

    setLoading(true);
    setError(null);

    try {
      const result = await analytics.aggregate<T>({
        table,
        aggregates: aggregates as { function: 'COUNT' | 'SUM' | 'AVG' | 'MIN' | 'MAX' | 'COUNT_DISTINCT'; column?: string; alias: string }[],
        ...queryOptions,
      });
      setData(result.data);
      setDuration(result.duration);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  }, [analytics, analyticsReady, enabled, table, aggregates, queryOptions]);

  useEffect(() => {
    runAggregate();
  }, [aggregateKey, analyticsReady]);

  return {
    data,
    loading,
    error,
    duration,
    refetch: runAggregate,
  };
}

/**
 * Hook to load data into local DuckDB and query it
 */
export function useLocalData<T = Record<string, unknown>>(
  tableName: string,
  initialData?: T[]
) {
  const { analytics, analyticsReady } = useGraphPost();
  const [ready, setReady] = useState(false);
  const [rowCount, setRowCount] = useState(0);
  const [error, setError] = useState<Error | null>(null);

  // Load initial data
  useEffect(() => {
    if (analyticsReady && analytics && initialData && initialData.length > 0) {
      analytics
        .loadFromJSON(tableName, initialData)
        .then((count) => {
          setRowCount(count);
          setReady(true);
        })
        .catch((err) => setError(err));
    }
  }, [analyticsReady, analytics, tableName, initialData]);

  const query = useCallback(
    async (sql: string) => {
      if (!analytics) throw new Error('Analytics not ready');
      return analytics.query<T>(sql);
    },
    [analytics]
  );

  const insert = useCallback(
    async (data: T[]) => {
      if (!analytics) throw new Error('Analytics not ready');
      const count = await analytics.insert(tableName, data as Record<string, unknown>[]);
      setRowCount((prev) => prev + count);
      return count;
    },
    [analytics, tableName]
  );

  return {
    ready: ready && analyticsReady,
    rowCount,
    error,
    query,
    insert,
  };
}

export default useGraphPostQuery;
