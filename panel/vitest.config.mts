import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  resolve: {
    tsconfigPaths: true,
    alias: {
      // The real module throws outside the react-server condition, which Next sets and Vitest doesn't.
      'server-only': fileURLToPath(new URL('node_modules/server-only/empty.js', import.meta.url)),
    },
  },
})
