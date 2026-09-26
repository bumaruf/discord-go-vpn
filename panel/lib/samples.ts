import { collection } from './mongodb'
import { COLLECTION, FIELD, SAMPLE_SCOPE, type ValueOf } from './mongo-names'

interface SampleDocument {
  [FIELD.timestamp]: Date
  [FIELD.meta]: { [FIELD.scope]: ValueOf<typeof SAMPLE_SCOPE>; [FIELD.identity]?: string }
  [FIELD.downBps]: number
  [FIELD.upBps]: number
}

export interface Sample {
  at: Date
  downBps: number
  upBps: number
}

/** `undefined` when no server sample is stored. */
export async function findLatestServerSample(): Promise<Sample | undefined> {
  const samples = await collection<SampleDocument>(COLLECTION.samples)
  const [latest] = await samples
    .find({ [`${FIELD.meta}.${FIELD.scope}`]: SAMPLE_SCOPE.server })
    .sort({ [FIELD.timestamp]: -1 })
    .limit(1)
    .toArray()

  if (!latest) return undefined
  return {
    at: latest[FIELD.timestamp],
    downBps: latest[FIELD.downBps],
    upBps: latest[FIELD.upBps],
  }
}
