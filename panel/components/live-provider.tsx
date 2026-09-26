'use client'

import { createContext, useContext } from 'react'
import { useLive, type LiveState } from '@/hooks/use-live'
import { EMPTY_SNAPSHOT } from '@/lib/live-snapshot'

const LiveContext = createContext<LiveState>({
  snapshot: EMPTY_SNAPSHOT,
  history: [],
  source: 'connecting',
})

/** Mounted in the panel layout, so navigation keeps one stream and its one-minute history. */
export function LiveProvider({ children }: { children: React.ReactNode }) {
  const state = useLive()
  return <LiveContext.Provider value={state}>{children}</LiveContext.Provider>
}

export function useLiveState(): LiveState {
  return useContext(LiveContext)
}
