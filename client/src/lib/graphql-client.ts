/**
 * GraphPost GraphQL Client
 *
 * Apollo Client setup with configurable logging and performance tracking.
 */

import {
  ApolloClient,
  InMemoryCache,
  HttpLink,
  ApolloLink,
  Observable,
  NormalizedCacheObject,
  DocumentNode,
  OperationVariables,
  TypedDocumentNode,
  FetchResult,
} from '@apollo/client';
import { getMainDefinition } from '@apollo/client/utilities';
import { getLogger, LoggerConfig } from './logger';

export interface GraphPostClientConfig {
  /** GraphQL endpoint URL */
  endpoint: string;

  /** Admin secret for authentication */
  adminSecret?: string;

  /** JWT token for authentication */
  jwtToken?: string;

  /** Custom headers */
  headers?: Record<string, string>;

  /** Enable Apollo DevTools */
  enableDevTools?: boolean;

  /** Logger configuration */
  loggerConfig?: Partial<LoggerConfig>;

  /** Cache configuration */
  cacheConfig?: {
    /** Type policies for cache normalization */
    typePolicies?: Record<string, unknown>;
    /** Enable query result caching */
    enabled?: boolean;
  };

  /** Request timeout in milliseconds */
  timeout?: number;

  /** Retry configuration */
  retry?: {
    maxRetries: number;
    retryDelay: number;
  };
}

const DEFAULT_CONFIG: Partial<GraphPostClientConfig> = {
  enableDevTools: process.env.NODE_ENV === 'development',
  timeout: 30000,
  retry: {
    maxRetries: 3,
    retryDelay: 1000,
  },
  cacheConfig: {
    enabled: true,
  },
};

/**
 * Create performance logging link for Apollo
 */
function createLoggingLink(config: GraphPostClientConfig): ApolloLink {
  const logger = getLogger(config.loggerConfig);

  return new ApolloLink((operation, forward) => {
    const startTime = performance.now();
    const { operationName } = operation;

    // Get operation type
    const definition = getMainDefinition(operation.query);
    const operationType =
      definition.kind === 'OperationDefinition'
        ? definition.operation
        : 'query';

    logger.debug('graphql', `Starting ${operationType}: ${operationName}`, {
      variables: operation.variables,
    });

    return new Observable((observer) => {
      const subscription = forward(operation).subscribe({
        next(result) {
          const duration = Math.round(performance.now() - startTime);
          const fromCache = result.data && (operation.getContext().fromCache || false);

          logger.logQuery({
            operationName,
            operationType: operationType as 'query' | 'mutation' | 'subscription',
            variables: operation.variables,
            duration,
            fromCache,
            message: '',
          });

          observer.next(result);
        },
        error(error) {
          const duration = Math.round(performance.now() - startTime);

          logger.logQuery({
            operationName,
            operationType: operationType as 'query' | 'mutation' | 'subscription',
            variables: operation.variables,
            duration,
            fromCache: false,
            error,
            message: '',
          });

          logger.error('graphql', `${operationType} failed: ${operationName}`, {
            error: error.message,
            duration,
          });

          observer.error(error);
        },
        complete() {
          observer.complete();
        },
      });

      return () => subscription.unsubscribe();
    });
  });
}

/**
 * Create authentication link
 */
function createAuthLink(config: GraphPostClientConfig): ApolloLink {
  return new ApolloLink((operation, forward) => {
    const headers: Record<string, string> = {
      ...config.headers,
    };

    if (config.adminSecret) {
      headers['x-hasura-admin-secret'] = config.adminSecret;
    }

    if (config.jwtToken) {
      headers['Authorization'] = `Bearer ${config.jwtToken}`;
    }

    operation.setContext({
      headers,
    });

    return forward(operation);
  });
}

/**
 * Create retry link for failed requests
 */
function createRetryLink(config: GraphPostClientConfig): ApolloLink {
  const { maxRetries = 3, retryDelay = 1000 } = config.retry || {};
  const logger = getLogger(config.loggerConfig);

  return new ApolloLink((operation, forward) => {
    let retryCount = 0;

    const tryRequest = (): Observable<FetchResult> => {
      return new Observable((observer) => {
        const subscription = forward(operation).subscribe({
          next: observer.next.bind(observer),
          error: (error) => {
            if (retryCount < maxRetries && isRetryableError(error)) {
              retryCount++;
              logger.warn('graphql', `Retrying request (${retryCount}/${maxRetries})`, {
                operation: operation.operationName,
                error: error.message,
              });

              setTimeout(() => {
                tryRequest().subscribe(observer);
              }, retryDelay * retryCount);
            } else {
              observer.error(error);
            }
          },
          complete: observer.complete.bind(observer),
        });

        return () => subscription.unsubscribe();
      });
    };

    return tryRequest();
  });
}

function isRetryableError(error: Error): boolean {
  // Retry on network errors and 5xx server errors
  const message = error.message.toLowerCase();
  return (
    message.includes('network') ||
    message.includes('fetch') ||
    message.includes('timeout') ||
    message.includes('500') ||
    message.includes('502') ||
    message.includes('503') ||
    message.includes('504')
  );
}

/**
 * Create the Apollo Client instance
 */
export function createGraphPostClient(
  config: GraphPostClientConfig
): ApolloClient<NormalizedCacheObject> {
  const mergedConfig = { ...DEFAULT_CONFIG, ...config };
  const logger = getLogger(mergedConfig.loggerConfig);

  logger.info('client', 'Initializing GraphPost client', {
    endpoint: mergedConfig.endpoint,
    cacheEnabled: mergedConfig.cacheConfig?.enabled,
  });

  // Create HTTP link
  const httpLink = new HttpLink({
    uri: mergedConfig.endpoint,
    fetchOptions: {
      timeout: mergedConfig.timeout,
    },
  });

  // Compose links
  const link = ApolloLink.from([
    createLoggingLink(mergedConfig),
    createRetryLink(mergedConfig),
    createAuthLink(mergedConfig),
    httpLink,
  ]);

  // Create cache
  const cache = new InMemoryCache({
    typePolicies: mergedConfig.cacheConfig?.typePolicies as Record<string, unknown>,
  });

  // Create client
  const client = new ApolloClient({
    link,
    cache,
    connectToDevTools: mergedConfig.enableDevTools,
    defaultOptions: {
      watchQuery: {
        fetchPolicy: mergedConfig.cacheConfig?.enabled ? 'cache-first' : 'network-only',
        errorPolicy: 'all',
      },
      query: {
        fetchPolicy: mergedConfig.cacheConfig?.enabled ? 'cache-first' : 'network-only',
        errorPolicy: 'all',
      },
      mutate: {
        errorPolicy: 'all',
      },
    },
  });

  return client;
}

// Type-safe query helpers
export type QueryResult<TData> = {
  data: TData | null;
  loading: boolean;
  error: Error | null;
};

/**
 * Execute a GraphQL query with logging
 */
export async function executeQuery<TData = unknown, TVariables extends OperationVariables = OperationVariables>(
  client: ApolloClient<NormalizedCacheObject>,
  query: DocumentNode | TypedDocumentNode<TData, TVariables>,
  variables?: TVariables
): Promise<QueryResult<TData>> {
  try {
    const result = await client.query<TData, TVariables>({
      query,
      variables,
    });

    return {
      data: result.data,
      loading: false,
      error: result.error || null,
    };
  } catch (error) {
    return {
      data: null,
      loading: false,
      error: error as Error,
    };
  }
}

/**
 * Execute a GraphQL mutation with logging
 */
export async function executeMutation<TData = unknown, TVariables extends OperationVariables = OperationVariables>(
  client: ApolloClient<NormalizedCacheObject>,
  mutation: DocumentNode | TypedDocumentNode<TData, TVariables>,
  variables?: TVariables
): Promise<QueryResult<TData>> {
  try {
    const result = await client.mutate<TData, TVariables>({
      mutation,
      variables,
    });

    return {
      data: result.data || null,
      loading: false,
      error: result.errors?.[0] ? new Error(result.errors[0].message) : null,
    };
  } catch (error) {
    return {
      data: null,
      loading: false,
      error: error as Error,
    };
  }
}

export default createGraphPostClient;
