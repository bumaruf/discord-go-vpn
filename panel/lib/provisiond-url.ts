import { provisiondBaseUrl } from './env'

/** Joins `path` to PROVISIOND_URL, tolerating a trailing slash; `undefined` when it is unset. */
export function provisiondUrl(path: string): string | undefined {
  const base = provisiondBaseUrl()
  return base && `${base.replace(/\/$/, '')}${path}`
}
