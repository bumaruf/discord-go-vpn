import type { NextConfig } from 'next'
import { initOpenNextCloudflareForDev } from '@opennextjs/cloudflare'

const nextConfig: NextConfig = {}

export default nextConfig

// Gives `next dev` the bindings in wrangler.jsonc. Dev only: proxy.test.ts imports this file, and
// the call there leaves a rejection behind outside Next.
if (process.env.NODE_ENV === 'development') {
  initOpenNextCloudflareForDev()
}
