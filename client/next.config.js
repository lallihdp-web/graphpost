/** @type {import('next').NextConfig} */
const nextConfig = {
  // Enable WebAssembly support for DuckDB-WASM
  webpack: (config, { isServer }) => {
    // Enable WebAssembly
    config.experiments = {
      ...config.experiments,
      asyncWebAssembly: true,
    };

    // Handle WASM files
    config.module.rules.push({
      test: /\.wasm$/,
      type: 'webassembly/async',
    });

    // Exclude DuckDB from server-side bundling
    if (isServer) {
      config.externals = [...(config.externals || []), '@duckdb/duckdb-wasm'];
    }

    return config;
  },

  // Allow loading WASM from CDN
  async headers() {
    return [
      {
        source: '/(.*)',
        headers: [
          {
            key: 'Cross-Origin-Opener-Policy',
            value: 'same-origin',
          },
          {
            key: 'Cross-Origin-Embedder-Policy',
            value: 'require-corp',
          },
        ],
      },
    ];
  },
};

module.exports = nextConfig;
