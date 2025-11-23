'use client';

import { useState } from 'react';
import { useGraphPost, useAnalytics, useLogger } from '@/lib/provider';
import QueryExplorer from './QueryExplorer';
import LogViewer from './LogViewer';
import AnalyticsPanel from './AnalyticsPanel';
import PerformanceReport from './PerformanceReport';

type Tab = 'explorer' | 'logs' | 'analytics' | 'performance';

export default function Dashboard() {
  const [activeTab, setActiveTab] = useState<Tab>('explorer');
  const { analyticsReady, initializeAnalytics } = useGraphPost();

  const tabs: { id: Tab; label: string }[] = [
    { id: 'explorer', label: 'Query Explorer' },
    { id: 'logs', label: 'Logs' },
    { id: 'analytics', label: 'Analytics' },
    { id: 'performance', label: 'Performance' },
  ];

  return (
    <div className="space-y-6">
      {/* Status Bar */}
      <div className="bg-white rounded-lg shadow p-4 flex items-center justify-between">
        <div className="flex items-center space-x-4">
          <StatusIndicator
            label="GraphQL"
            status="connected"
          />
          <StatusIndicator
            label="DuckDB-WASM"
            status={analyticsReady ? 'connected' : 'disconnected'}
          />
        </div>
        {!analyticsReady && (
          <button
            onClick={() => initializeAnalytics()}
            className="px-4 py-2 bg-graphpost-600 text-white rounded-lg hover:bg-graphpost-700 transition"
          >
            Initialize Analytics
          </button>
        )}
      </div>

      {/* Tab Navigation */}
      <div className="border-b border-gray-200">
        <nav className="flex space-x-8">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`py-4 px-1 border-b-2 font-medium text-sm transition ${
                activeTab === tab.id
                  ? 'border-graphpost-500 text-graphpost-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab Content */}
      <div className="bg-white rounded-lg shadow">
        {activeTab === 'explorer' && <QueryExplorer />}
        {activeTab === 'logs' && <LogViewer />}
        {activeTab === 'analytics' && <AnalyticsPanel />}
        {activeTab === 'performance' && <PerformanceReport />}
      </div>
    </div>
  );
}

function StatusIndicator({ label, status }: { label: string; status: 'connected' | 'disconnected' }) {
  return (
    <div className="flex items-center space-x-2">
      <div
        className={`w-3 h-3 rounded-full ${
          status === 'connected' ? 'bg-green-500' : 'bg-gray-400'
        }`}
      />
      <span className="text-sm text-gray-600">{label}</span>
    </div>
  );
}
