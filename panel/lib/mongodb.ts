import 'server-only'

import { getCloudflareContext } from '@opennextjs/cloudflare'
import {
  BSON,
  type DeleteResult,
  type Document,
  type Filter,
  type FindOptions,
  type InsertOneResult,
  type MongoClient,
  type OptionalUnlessRequiredId,
  type Sort,
  type UpdateFilter,
  type UpdateOptions,
  type UpdateResult,
  type WithId,
} from 'mongodb'
import { mongodbDb } from './env'
import type { COLLECTION, ValueOf } from './mongo-names'
import { connectMongo, runOperation, type MongoOperation } from './mongo-operation'

/** The part of a driver cursor the repositories use. */
export interface Cursor<T> {
  sort(sort: Sort): Cursor<T>
  limit(limit: number): Cursor<T>
  toArray(): Promise<T[]>
}

/** The part of the driver's Collection the repositories use; a real Collection satisfies it. */
export interface MongoCollection<TSchema extends Document> {
  find(filter: Filter<TSchema>, options?: FindOptions): Cursor<WithId<TSchema>>
  findOne(filter: Filter<TSchema>): Promise<WithId<TSchema> | null>
  aggregate<T extends Document>(pipeline: Document[]): { toArray(): Promise<T[]> }
  insertOne(document: OptionalUnlessRequiredId<TSchema>): Promise<InsertOneResult<TSchema>>
  updateOne(
    filter: Filter<TSchema>,
    update: UpdateFilter<TSchema>,
    options?: UpdateOptions,
  ): Promise<UpdateResult<TSchema>>
  deleteOne(filter: Filter<TSchema>): Promise<DeleteResult>
}

declare global {
  /** Node only: on Cloudflare the client lives in the MongoGateway Durable Object. */
  var _mongoClientPromise: Promise<MongoClient> | undefined
}

/**
 * Kept on `globalThis`, which survives across requests of a warm Node process: a client per
 * request would exhaust the cluster's connection pool within minutes.
 */
function nodeClient(): Promise<MongoClient> {
  globalThis._mongoClientPromise ??= connectMongo().catch((error) => {
    globalThis._mongoClientPromise = undefined
    throw error
  })
  return globalThis._mongoClientPromise
}

/** Typed by the caller: the result has whatever shape the driver returns for that operation. */
async function send<T>(operation: MongoOperation): Promise<T> {
  // `next dev` runs on Node, where the driver is called directly.
  if (navigator.userAgent !== 'Cloudflare-Workers') {
    const client = await nodeClient()
    return (await runOperation(client.db(mongodbDb()), operation)) as T
  }

  const gateways = getCloudflareContext().env.MONGO_GATEWAY
  const gateway = gateways.get(gateways.idFromName('primary'))
  const result = await gateway.run(BSON.EJSON.stringify(operation, { relaxed: false }))
  return BSON.EJSON.parse(result, { relaxed: false }) as T
}

function cursor<T>(collection: string, filter: Document, options: FindOptions = {}): Cursor<T> {
  return {
    sort: (sort) => cursor(collection, filter, { ...options, sort }),
    limit: (limit) => cursor(collection, filter, { ...options, limit }),
    toArray: () => send({ collection, op: 'find', filter, options }),
  }
}

export async function collection<TSchema extends Document>(
  name: ValueOf<typeof COLLECTION>,
): Promise<MongoCollection<TSchema>> {
  return {
    find: (filter, options) => cursor(name, filter, options),
    findOne: (filter) => send({ collection: name, op: 'findOne', filter }),
    aggregate: (pipeline) => ({
      toArray: () => send({ collection: name, op: 'aggregate', pipeline }),
    }),
    insertOne: (document) => send({ collection: name, op: 'insertOne', document }),
    updateOne: (filter, update, options) =>
      send({ collection: name, op: 'updateOne', filter, update, options }),
    deleteOne: (filter) => send({ collection: name, op: 'deleteOne', filter }),
  }
}
