import 'server-only'

import { MongoClient, type Collection, type Document } from 'mongodb'
import { mongodbDb, mongodbUri } from './env'
import type { COLLECTION, ValueOf } from './mongo-names'

/**
 * Kept on `globalThis`, which survives across invocations of a warm serverless instance: a client
 * per invocation would exhaust the cluster's connection pool within minutes.
 */
declare global {
  var _mongoClientPromise: Promise<MongoClient> | undefined
}

function clientPromise(): Promise<MongoClient> {
  const uri = mongodbUri()
  if (!uri) {
    throw new Error('set MONGODB_URI to the Atlas connection string')
  }

  globalThis._mongoClientPromise ??= new MongoClient(uri, {
    maxPoolSize: 10,
    // Fail fast: saying the database didn't answer beats a page stuck waiting for it.
    serverSelectionTimeoutMS: 5000,
  }).connect()

  return globalThis._mongoClientPromise
}

export async function collection<TSchema extends Document>(
  name: ValueOf<typeof COLLECTION>,
): Promise<Collection<TSchema>> {
  const client = await clientPromise()
  return client.db(mongodbDb()).collection<TSchema>(name)
}
