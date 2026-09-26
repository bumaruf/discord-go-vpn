import { MongoClient } from 'mongodb'
import { mongodbDb, mongodbUri } from '../lib/env'
import { mergeDevicesWithoutInvite } from '../lib/merge-devices'

async function main() {
  const uri = mongodbUri()
  if (!uri) {
    console.error('set MONGODB_URI (in panel/.env.local) before merging devices')
    process.exit(1)
  }

  const client = new MongoClient(uri, { serverSelectionTimeoutMS: 8000 })
  await client.connect()

  console.log('==> merging the devices the removed global token left')
  const { merged, waiting, ambiguous } = await mergeDevicesWithoutInvite(client.db(mongodbDb()))

  for (const m of merged) {
    console.log(
      `    ${machine(m)}: ${m.days} day(s) and ${m.sessions} session(s) moved to ${m.into}`,
    )
  }
  if (merged.length === 0) console.log('    nothing to merge')

  if (waiting.length > 0) {
    console.log('\n==> left alone: the machine has not leased through an invite yet')
    console.log('    provisiond adopts each of these on its next lease, keeping its history')
    for (const w of waiting) console.log(`    ${machine(w)}: ${w.id}`)
  }

  if (ambiguous.length > 0) {
    console.log(
      '\n==> left alone: the same name and os user is registered under more than one invite',
    )
    console.log('    merging would hand one machine the history of another, so pick by hand')
    for (const a of ambiguous) {
      console.log(`    ${machine(a)}: ${a.id} matches ${a.candidates.join(', ')}`)
    }
  }

  console.log('\n==> done')
  await client.close()
}

function machine({ name, osUser }: { name: string; osUser: string | null }): string {
  return osUser ? `${name || '(unnamed)'}/${osUser}` : name || '(unnamed)'
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
