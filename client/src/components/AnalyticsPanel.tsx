'use client';

import { useState } from 'react';
import { useAnalytics } from '@/lib/provider';

export default function AnalyticsPanel() {
  const { analytics, ready, initialize } = useAnalytics();
  const [sqlQuery, setSqlQuery] = useState('SELECT * FROM _query_metrics LIMIT 10');
  const [result, setResult] = useState<unknown[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [duration, setDuration] = useState<number>(0);

  // Sample data for demonstration
  const [sampleData] = useState([
    { id: 1, product: 'Widget A', category: 'Electronics', sales: 150, revenue: 4500 },
    { id: 2, product: 'Widget B', category: 'Electronics', sales: 230, revenue: 6900 },
    { id: 3, product: 'Gadget X', category: 'Accessories', sales: 80, revenue: 2400 },
    { id: 4, product: 'Gadget Y', category: 'Accessories', sales: 120, revenue: 3600 },
    { id: 5, product: 'Device Z', category: 'Electronics', sales: 300, revenue: 15000 },
  ]);

  const executeQuery = async () => {
    if (!analytics) {
      setError('Analytics not initialized');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const queryResult = await analytics.query(sqlQuery);
      setResult(queryResult.data);
      setDuration(queryResult.duration);
    } catch (err) {
      setError((err as Error).message);
      setResult([]);
    } finally {
      setLoading(false);
    }
  };

  const loadSampleData = async () => {
    if (!analytics) return;

    try {
      await analytics.loadFromJSON('sample_sales', sampleData);
      setSqlQuery(`SELECT
  category,
  COUNT(*) as product_count,
  SUM(sales) as total_sales,
  SUM(revenue) as total_revenue,
  AVG(revenue) as avg_revenue
FROM sample_sales
GROUP BY category`);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  if (!ready) {
    return (
      <div className="p-6">
        <h2 className="text-xl font-semibold mb-4">Client-Side Analytics (DuckDB-WASM)</h2>
        <div className="bg-amber-50 border border-amber-200 rounded-lg p-6 text-center">
          <p className="text-amber-800 mb-4">
            DuckDB-WASM is not initialized. Initialize it to enable client-side analytics.
          </p>
          <button
            onClick={() => initialize()}
            className="px-4 py-2 bg-graphpost-600 text-white rounded-lg hover:bg-graphpost-700 transition"
          >
            Initialize DuckDB-WASM
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-xl font-semibold">Client-Side Analytics (DuckDB-WASM)</h2>
        <button
          onClick={loadSampleData}
          className="px-3 py-1 text-sm bg-graphpost-100 text-graphpost-700 rounded hover:bg-graphpost-200 transition"
        >
          Load Sample Data
        </button>
      </div>

      <div className="grid grid-cols-2 gap-6">
        {/* SQL Editor */}
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              SQL Query (DuckDB)
            </label>
            <textarea
              value={sqlQuery}
              onChange={(e) => setSqlQuery(e.target.value)}
              className="w-full h-40 font-mono text-sm p-4 border rounded-lg focus:ring-2 focus:ring-graphpost-500 focus:border-graphpost-500"
              placeholder="Enter SQL query..."
            />
          </div>

          <button
            onClick={executeQuery}
            disabled={loading}
            className="w-full py-2 px-4 bg-graphpost-600 text-white rounded-lg hover:bg-graphpost-700 disabled:bg-gray-400 transition"
          >
            {loading ? 'Executing...' : 'Execute SQL'}
          </button>

          {/* Quick Queries */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Quick Queries
            </label>
            <div className="flex flex-wrap gap-2">
              <QuickQueryButton
                label="Query Metrics"
                query="SELECT * FROM _query_metrics ORDER BY executed_at DESC LIMIT 10"
                onClick={setSqlQuery}
              />
              <QuickQueryButton
                label="Slow Queries"
                query="SELECT query_name, duration_ms FROM _query_metrics WHERE duration_ms > 500 ORDER BY duration_ms DESC"
                onClick={setSqlQuery}
              />
              <QuickQueryButton
                label="Cache Stats"
                query="SELECT query_name, from_cache, COUNT(*) as count FROM _query_metrics GROUP BY query_name, from_cache"
                onClick={setSqlQuery}
              />
            </div>
          </div>
        </div>

        {/* Results */}
        <div className="space-y-4">
          <div className="flex justify-between items-center">
            <label className="block text-sm font-medium text-gray-700">
              Results
            </label>
            {duration > 0 && (
              <span className="text-xs text-gray-500">
                Executed in {duration}ms
              </span>
            )}
          </div>

          <div className="border rounded-lg overflow-hidden">
            {error ? (
              <div className="p-4 bg-red-50 text-red-700 text-sm">{error}</div>
            ) : result.length > 0 ? (
              <div className="overflow-auto max-h-72">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr>
                      {Object.keys(result[0] as Record<string, unknown>).map((key) => (
                        <th
                          key={key}
                          className="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase"
                        >
                          {key}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {result.map((row, i) => (
                      <tr key={i} className="hover:bg-gray-50">
                        {Object.values(row as Record<string, unknown>).map((val, j) => (
                          <td key={j} className="px-3 py-2 text-sm text-gray-600">
                            {formatValue(val)}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="p-8 text-center text-gray-500 text-sm">
                No results. Execute a query to see data.
              </div>
            )}
          </div>

          <div className="text-sm text-gray-500">
            {result.length} row{result.length !== 1 ? 's' : ''} returned
          </div>
        </div>
      </div>
    </div>
  );
}

function QuickQueryButton({
  label,
  query,
  onClick,
}: {
  label: string;
  query: string;
  onClick: (query: string) => void;
}) {
  return (
    <button
      onClick={() => onClick(query)}
      className="px-2 py-1 text-xs bg-gray-100 text-gray-700 rounded hover:bg-gray-200 transition"
    >
      {label}
    </button>
  );
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined) return '-';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (typeof value === 'number') return value.toLocaleString();
  if (value instanceof Date) return value.toLocaleString();
  return String(value);
}
