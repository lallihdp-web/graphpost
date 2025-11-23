'use client';

import { GraphPostProvider } from '@/lib/provider';
import Dashboard from '@/components/Dashboard';

const GRAPHPOST_CONFIG = {
  endpoint: process.env.NEXT_PUBLIC_GRAPHPOST_ENDPOINT || 'http://localhost:8080/v1/graphql',
  adminSecret: process.env.NEXT_PUBLIC_GRAPHPOST_ADMIN_SECRET,
  loggerConfig: {
    level: 'debug' as const,
    queryLogging: true,
    slowQueryThreshold: 1000,
    consoleOutput: true,
    collectMetrics: true,
  },
};

export default function Home() {
  return (
    <GraphPostProvider config={GRAPHPOST_CONFIG} autoInitAnalytics>
      <main className="min-h-screen p-8">
        <header className="mb-8">
          <h1 className="text-3xl font-bold text-gray-900">GraphPost Client</h1>
          <p className="text-gray-600 mt-2">
            Next.js GraphQL client with DuckDB-WASM analytics and configurable logging
          </p>
        </header>
        <Dashboard />
      </main>
    </GraphPostProvider>
  );
}
