import { DurableObject } from 'cloudflare:workers'
import { BSON, type MongoClient } from 'mongodb'
import { mongodbDb } from './env'
import { connectMongo, runOperation, type MongoOperation } from './mongo-operation'

/**
 * Holds the panel's only MongoClient on Cloudflare. A Worker can't reuse a socket opened by
 * another request, so a client per request would pay Atlas's TLS and auth handshake on every
 * page; a Durable Object may keep its connections between requests.
 *
 * Operations and results cross the RPC boundary as canonical EJSON: structured clone would turn
 * an ObjectId into a plain object.
 */
export class MongoGateway extends DurableObject {
  private client?: Promise<MongoClient>

  async run(operation: string): Promise<string> {
    const client = await this.mongo()
    const parsed = BSON.EJSON.parse(operation, { relaxed: false }) as MongoOperation
    const result = await runOperation(client.db(mongodbDb()), parsed)
    return BSON.EJSON.stringify(result, { relaxed: false })
  }

  private mongo(): Promise<MongoClient> {
    this.client ??= connectMongo().catch((error) => {
      // Not cached: the next operation retries instead of failing until the object is evicted.
      this.client = undefined
      throw error
    })
    return this.client
  }
}
