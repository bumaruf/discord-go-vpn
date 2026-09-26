import { MongoClient } from 'mongodb'
import { mongodbDb, mongodbUri } from '../lib/env'
import { ensureIndexes } from '../lib/indexes'

async function main() {
  const uri = mongodbUri()
  if (!uri) {
    console.error('set MONGODB_URI (in panel/.env.local) before running setup')
    process.exit(1)
  }

  const client = new MongoClient(uri, { serverSelectionTimeoutMS: 8000 })
  await client.connect()
  const db = client.db(mongodbDb())

  console.log('==> creating collections and indexes')
  for (const line of await ensureIndexes(db)) console.log(`    ${line}`)

  console.log('\n==> done')
  console.log('    panel login comes from ADMIN_USERNAME and ADMIN_PASSWORD_HASH,')
  console.log(
    '    not from the database. Generate the hash with: npm run hash-password -- "password"',
  )

  await client.close()
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
