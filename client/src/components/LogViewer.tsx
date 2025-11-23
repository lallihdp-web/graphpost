'use client';

import { useState, useEffect } from 'react';
import { useLogger } from '@/lib/provider';
import { LogLevel } from '@/lib/logger';

export default function LogViewer() {
  const logger = useLogger();
  const [logs, setLogs] = useState(logger.getLogs());
  const [filter, setFilter] = useState<LogLevel | 'all'>('all');
  const [category, setCategory] = useState<string>('all');
  const [autoRefresh, setAutoRefresh] = useState(true);

  useEffect(() => {
    if (!autoRefresh) return;

    const interval = setInterval(() => {
      const newLogs = logger.getLogs(
        filter !== 'all' || category !== 'all'
          ? {
              level: filter !== 'all' ? filter : undefined,
              category: category !== 'all' ? category : undefined,
            }
          : undefined
      );
      setLogs(newLogs);
    }, 1000);

    return () => clearInterval(interval);
  }, [logger, filter, category, autoRefresh]);

  const handleExport = () => {
    const exportData = logger.exportLogs();
    const blob = new Blob([exportData], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `graphpost-logs-${new Date().toISOString()}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const categories = ['all', ...new Set(logs.map((l) => l.category))];

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-xl font-semibold">Query Logs</h2>
        <div className="flex items-center space-x-4">
          <label className="flex items-center space-x-2">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
              className="rounded text-graphpost-600"
            />
            <span className="text-sm">Auto-refresh</span>
          </label>
          <button
            onClick={() => logger.clearLogs()}
            className="px-3 py-1 text-sm bg-red-100 text-red-700 rounded hover:bg-red-200 transition"
          >
            Clear Logs
          </button>
          <button
            onClick={handleExport}
            className="px-3 py-1 text-sm bg-graphpost-100 text-graphpost-700 rounded hover:bg-graphpost-200 transition"
          >
            Export
          </button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex space-x-4 mb-4">
        <select
          value={filter}
          onChange={(e) => setFilter(e.target.value as LogLevel | 'all')}
          className="px-3 py-2 border rounded-lg text-sm focus:ring-2 focus:ring-graphpost-500"
        >
          <option value="all">All Levels</option>
          <option value="debug">Debug</option>
          <option value="info">Info</option>
          <option value="warn">Warning</option>
          <option value="error">Error</option>
        </select>

        <select
          value={category}
          onChange={(e) => setCategory(e.target.value)}
          className="px-3 py-2 border rounded-lg text-sm focus:ring-2 focus:ring-graphpost-500"
        >
          {categories.map((cat) => (
            <option key={cat} value={cat}>
              {cat === 'all' ? 'All Categories' : cat}
            </option>
          ))}
        </select>
      </div>

      {/* Log Entries */}
      <div className="border rounded-lg overflow-hidden">
        <div className="max-h-96 overflow-auto">
          {logs.length === 0 ? (
            <div className="p-8 text-center text-gray-500">
              No logs yet. Execute some queries to see logs here.
            </div>
          ) : (
            <table className="w-full">
              <thead className="bg-gray-50 sticky top-0">
                <tr>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Time
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Level
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Category
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Duration
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">
                    Message
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {logs.slice(-100).reverse().map((log, index) => (
                  <tr key={index} className="hover:bg-gray-50">
                    <td className="px-4 py-2 text-xs text-gray-500 whitespace-nowrap">
                      {new Date(log.timestamp).toLocaleTimeString()}
                    </td>
                    <td className="px-4 py-2">
                      <span
                        className={`px-2 py-1 text-xs rounded-full ${getLevelStyle(
                          log.level
                        )}`}
                      >
                        {log.level}
                      </span>
                    </td>
                    <td className="px-4 py-2 text-sm text-gray-600">
                      {log.category}
                    </td>
                    <td className="px-4 py-2 text-sm text-gray-600">
                      {log.duration ? `${log.duration}ms` : '-'}
                    </td>
                    <td className="px-4 py-2 text-sm text-gray-800 max-w-md truncate">
                      {log.message}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      <div className="mt-2 text-sm text-gray-500">
        Showing {Math.min(logs.length, 100)} of {logs.length} log entries
      </div>
    </div>
  );
}

function getLevelStyle(level: string): string {
  switch (level) {
    case 'debug':
      return 'bg-gray-100 text-gray-700';
    case 'info':
      return 'bg-blue-100 text-blue-700';
    case 'warn':
      return 'bg-amber-100 text-amber-700';
    case 'error':
      return 'bg-red-100 text-red-700';
    default:
      return 'bg-gray-100 text-gray-700';
  }
}
