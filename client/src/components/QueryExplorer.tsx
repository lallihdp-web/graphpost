'use client';

import { useState } from 'react';
import { gql, useQuery } from '@apollo/client';
import { useGraphPost } from '@/lib/provider';

const INTROSPECTION_QUERY = gql`
  query IntrospectionQuery {
    __schema {
      queryType {
        name
        fields {
          name
          description
          type {
            name
            kind
          }
        }
      }
      mutationType {
        name
        fields {
          name
          description
        }
      }
      types {
        name
        kind
        description
      }
    }
  }
`;

export default function QueryExplorer() {
  const [query, setQuery] = useState(`query {
  __typename
}`);
  const [variables, setVariables] = useState('{}');
  const [result, setResult] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { client } = useGraphPost();

  const { data: schemaData } = useQuery(INTROSPECTION_QUERY, {
    fetchPolicy: 'cache-first',
  });

  const executeQuery = async () => {
    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const parsedVariables = JSON.parse(variables);
      const response = await client.query({
        query: gql(query),
        variables: parsedVariables,
        fetchPolicy: 'network-only',
      });
      setResult(response.data);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const queryTypes = schemaData?.__schema?.queryType?.fields || [];

  return (
    <div className="p-6">
      <h2 className="text-xl font-semibold mb-4">Query Explorer</h2>

      <div className="grid grid-cols-2 gap-6">
        {/* Query Editor */}
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              GraphQL Query
            </label>
            <textarea
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              className="w-full h-48 font-mono text-sm p-4 border rounded-lg focus:ring-2 focus:ring-graphpost-500 focus:border-graphpost-500"
              placeholder="Enter your GraphQL query..."
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Variables (JSON)
            </label>
            <textarea
              value={variables}
              onChange={(e) => setVariables(e.target.value)}
              className="w-full h-24 font-mono text-sm p-4 border rounded-lg focus:ring-2 focus:ring-graphpost-500 focus:border-graphpost-500"
              placeholder="{}"
            />
          </div>

          <button
            onClick={executeQuery}
            disabled={loading}
            className="w-full py-2 px-4 bg-graphpost-600 text-white rounded-lg hover:bg-graphpost-700 disabled:bg-gray-400 transition"
          >
            {loading ? 'Executing...' : 'Execute Query'}
          </button>
        </div>

        {/* Result Panel */}
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Result
            </label>
            <div className="h-48 overflow-auto bg-gray-900 rounded-lg p-4">
              {error ? (
                <pre className="text-red-400 text-sm">{error}</pre>
              ) : result ? (
                <pre className="text-gray-100 text-sm">
                  {JSON.stringify(result, null, 2)}
                </pre>
              ) : (
                <span className="text-gray-500 text-sm">
                  Execute a query to see results...
                </span>
              )}
            </div>
          </div>

          {/* Available Types */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Available Query Fields
            </label>
            <div className="h-32 overflow-auto border rounded-lg p-4 bg-gray-50">
              {queryTypes.length > 0 ? (
                <ul className="space-y-1">
                  {queryTypes.slice(0, 10).map((field: { name: string; type: { name: string } }) => (
                    <li key={field.name} className="text-sm">
                      <span className="font-medium text-graphpost-600">{field.name}</span>
                      <span className="text-gray-500">: {field.type?.name || 'Unknown'}</span>
                    </li>
                  ))}
                  {queryTypes.length > 10 && (
                    <li className="text-sm text-gray-400">
                      ... and {queryTypes.length - 10} more
                    </li>
                  )}
                </ul>
              ) : (
                <span className="text-gray-500 text-sm">Loading schema...</span>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
