/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineConfig } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

/**
 * Builds the API documentation page as a fully static, backend-independent
 * bundle. `assetPrefix` is set via ASSET_PREFIX (default `/apidocs/`) so the
 * output can be mounted at a non-conflicting path behind a reverse proxy.
 */
export default defineConfig({
  plugins: [pluginReact(), pluginTailwindcss({ optimize: true })],
  source: {
    entry: {
      index: './src/standalone/docs-standalone.tsx',
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  html: {
    template: './src/standalone/template.html',
    title: 'Nexus Reach Seedance 2.0 API Docs',
  },
  output: {
    minify: true,
    target: 'web',
    assetPrefix: process.env.ASSET_PREFIX || '/apidocs/',
    distPath: {
      root: 'dist-docs',
    },
  },
  // Split React and other vendor code into cacheable chunks so repeat visits
  // and parallel downloads stay fast.
  splitChunks: {
    preset: 'default',
    cacheGroups: {
      'vendor-react': {
        test: /node_modules[\\/](react|react-dom)[\\/]/,
        name: 'vendor-react',
        chunks: 'all',
        priority: 10,
        enforce: true,
      },
    },
  },
  performance: {
    removeConsole: ['log'],
    buildCache: false,
  },
})
