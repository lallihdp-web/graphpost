'use client';

import { useState, useEffect } from 'react';
import { useLogger, useAnalytics } from '@/lib/provider';
import { SlowQueryReport } from '@/lib/logger';

export default function PerformanceReport() {
  const logger = useLogger();
  const { analytics, ready } = useAnalytics();
  const [report, setReport] = useState<SlowQueryReport[]>([]);
  const [duckdbReport, setDuckdbReport] = useState<unknown[]>([]);

  useEffect(() => {
    // Update report every 5 seconds
    const interval = setInterval(() => {
      setReport(logger.getSlowQueryReport());
    }, 5000);

    setReport(logger.getSlowQueryReport());
    return () => clearInterval(interval);
  }, [logger]);

  useEffect(() => {
    if (ready && analytics) {
      analytics.getPerformanceReport().then((result) => {
        setDuckdbReport(result.data);
      });
    }
  }, [ready, analytics]);

  const configureLogging = (threshold: number) => {
    logger.configure({ slowQueryThreshold: threshold });
  };

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <h2 className="text-xl font-semibold">Performance Analysis</h2>
        <div className="flex items-center space-x-4">
          <span className="text-sm text-gray-600">Slow Query Threshold:</span>
          <select
            onChange={(e) => configureLogging(Number(e.target.value))}
            className="px-3 py-1 border rounded text-sm"
            defaultValue="1000"
          >
            <option value="500">500ms</option>
            <option value="1000">1s</option>
            <option value="2000">2s</option>
            <option value="5000">5s</option>
          </select>
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <MetricCard
          title="Slow Queries"
          value={report.length}
          description="Queries exceeding threshold"
          color="red"
        />
        <MetricCard
          title="Avg Duration"
          value={`${Math.round(report.reduce((sum, r) => sum + r.averageDuration, 0) / (report.length || 1))}ms`}
          description="Average query time"
          color="amber"
        />
        <MetricCard
          title="Max Duration"
          value={`${Math.max(...report.map((r) => r.maxDuration), 0)}ms`}
          description="Slowest query"
          color="orange"
        />
        <MetricCard
          title="Total Queries"
          value={report.reduce((sum, r) => sum + r.count, 0)}
          description="Queries analyzed"
          color="blue"
        />
      </div>

      {/* Slow Query Report */}
      <div className="bg-white border rounded-lg overflow-hidden mb-6">
        <div className="px-4 py-3 bg-gray-50 border-b">
          <h3 className="font-medium">Slow Query Report</h3>
          <p className="text-sm text-gray-500">
            Queries that need optimization attention
          </p>
        </div>

        {report.length === 0 ? (
          <div className="p-8 text-center text-gray-500">
            <p>No slow queries detected yet.</p>
            <p className="text-sm mt-2">
              Execute some queries to see performance analysis.
            </p>
          </div>
        ) : (
          <div className="overflow-auto">
            <table className="w-full">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Operation
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Avg Duration
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    P95 Duration
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Count
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Suggestions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {report.map((item, index) => (
                  <tr key={index} className="hover:bg-gray-50">
                    <td className="px-4 py-3">
                      <span className="font-mono text-sm">
                        {item.operationName}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <DurationBadge duration={item.averageDuration} />
                    </td>
                    <td className="px-4 py-3">
                      <DurationBadge duration={item.p95Duration} />
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-600">
                      {item.count}
                    </td>
                    <td className="px-4 py-3">
                      <ul className="text-xs text-gray-600 space-y-1">
                        {item.suggestions.slice(0, 2).map((suggestion, i) => (
                          <li key={i} className="flex items-start">
                            <span className="text-amber-500 mr-1">•</span>
                            {suggestion}
                          </li>
                        ))}
                      </ul>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* DuckDB Analytics Report */}
      {ready && duckdbReport.length > 0 && (
        <div className="bg-white border rounded-lg overflow-hidden">
          <div className="px-4 py-3 bg-gray-50 border-b">
            <h3 className="font-medium">DuckDB Performance Metrics</h3>
            <p className="text-sm text-gray-500">
              Client-side query performance tracking
            </p>
          </div>

          <div className="overflow-auto">
            <table className="w-full">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Query
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Avg Duration
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Total Calls
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Cache Hit Rate
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {duckdbReport.map((item: unknown, index) => {
                  const row = item as Record<string, unknown>;
                  return (
                    <tr key={index} className="hover:bg-gray-50">
                      <td className="px-4 py-2 font-mono text-sm">
                        {String(row.query_name)}
                      </td>
                      <td className="px-4 py-2 text-sm">
                        {Number(row.avg_duration).toFixed(2)}ms
                      </td>
                      <td className="px-4 py-2 text-sm">
                        {String(row.total_calls)}
                      </td>
                      <td className="px-4 py-2">
                        <CacheHitBadge rate={Number(row.cache_hit_rate)} />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Optimization Tips */}
      <div className="mt-6 bg-blue-50 border border-blue-200 rounded-lg p-4">
        <h3 className="font-medium text-blue-800 mb-2">Optimization Tips</h3>
        <ul className="text-sm text-blue-700 space-y-1">
          <li>• Enable query caching for frequently accessed data</li>
          <li>• Use DuckDB analytics for complex aggregate queries</li>
          <li>• Add database indexes for columns used in WHERE clauses</li>
          <li>• Paginate large result sets to reduce response time</li>
          <li>• Consider materialized aggregates for dashboard metrics</li>
        </ul>
      </div>
    </div>
  );
}

function MetricCard({
  title,
  value,
  description,
  color,
}: {
  title: string;
  value: string | number;
  description: string;
  color: 'red' | 'amber' | 'orange' | 'blue' | 'green';
}) {
  const colorStyles = {
    red: 'bg-red-50 border-red-200',
    amber: 'bg-amber-50 border-amber-200',
    orange: 'bg-orange-50 border-orange-200',
    blue: 'bg-blue-50 border-blue-200',
    green: 'bg-green-50 border-green-200',
  };

  return (
    <div className={`border rounded-lg p-4 ${colorStyles[color]}`}>
      <div className="text-sm font-medium text-gray-600">{title}</div>
      <div className="text-2xl font-bold mt-1">{value}</div>
      <div className="text-xs text-gray-500 mt-1">{description}</div>
    </div>
  );
}

function DurationBadge({ duration }: { duration: number }) {
  const color =
    duration > 3000
      ? 'bg-red-100 text-red-700'
      : duration > 1000
        ? 'bg-amber-100 text-amber-700'
        : 'bg-green-100 text-green-700';

  return (
    <span className={`px-2 py-1 text-xs rounded ${color}`}>
      {duration}ms
    </span>
  );
}

function CacheHitBadge({ rate }: { rate: number }) {
  const color =
    rate > 80
      ? 'bg-green-100 text-green-700'
      : rate > 50
        ? 'bg-amber-100 text-amber-700'
        : 'bg-red-100 text-red-700';

  return (
    <span className={`px-2 py-1 text-xs rounded ${color}`}>
      {rate.toFixed(1)}%
    </span>
  );
}
