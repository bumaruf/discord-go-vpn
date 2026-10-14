import { MongoClient, type Db, type Document, type FindOptions, type UpdateOptions } from 'mongodb'
import { mongodbUri } from './env'

/** One driver call, as the repositories make it: plain data, so it can cross an RPC boundary. */
export type MongoOperation = { collection: string } & (
  | { op: 'find'; filter: Document; options: FindOptions }
  | { op: 'findOne'; filter: Document }
  | { op: 'aggregate'; pipeline: Document[] }
  | { op: 'insertOne'; document: Document }
  | { op: 'updateOne'; filter: Document; update: Document; options?: UpdateOptions }
  | { op: 'deleteOne'; filter: Document }
)

export function runOperation(db: Db, operation: MongoOperation): Promise<unknown> {
  const coll = db.collection(operation.collection)
  switch (operation.op) {
    case 'find':
      return coll.find(operation.filter, operation.options).toArray()
    case 'findOne':
      return coll.findOne(operation.filter)
    case 'aggregate':
      return coll.aggregate(operation.pipeline).toArray()
    case 'insertOne':
      return coll.insertOne(operation.document)
    case 'updateOne':
      return coll.updateOne(operation.filter, operation.update, operation.options)
    case 'deleteOne':
      return coll.deleteOne(operation.filter)
  }
}

export function connectMongo(): Promise<MongoClient> {
  const uri = mongodbUri()
  if (!uri) {
    throw new Error('set MONGODB_URI to the Atlas connection string')
  }

  return new MongoClient(uri, {
    maxPoolSize: 10,
    // Fail fast: saying the database didn't answer beats a page stuck waiting for it.
    serverSelectionTimeoutMS: 5000,
  }).connect()
}
