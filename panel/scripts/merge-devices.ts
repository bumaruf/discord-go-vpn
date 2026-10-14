import { MongoClient } from 'mongodb'
import type { DeviceDocument } from '../lib/documents'
import { mongodbDb, mongodbUri } from '../lib/env'
import { mergeDevices, planDeviceMerges } from '../lib/merge-devices'
import { FIELD } from '../lib/mongo-names'

async function main() {
  const uri = mongodbUri()
  if (!uri) {
    console.error('set MONGODB_URI (in panel/.env.local) before merging devices')
    process.exit(1)
  }
  const dryRun = process.argv.includes('--dry-run')

  const client = new MongoClient(uri, { serverSelectionTimeoutMS: 8000 })
  await client.connect()
  const db = client.db(mongodbDb())

  console.log(`==> devices of the same machine${dryRun ? ' (dry run: nothing is written)' : ''}`)
  const plan = await planDeviceMerges(db)

  for (const { into, from } of plan.machines) {
    console.log(`    ${machine(into)}: keeps ${describe(into)}`)
    for (const device of from) console.log(`      merges ${describe(device)}`)
  }
  if (plan.machines.length === 0) console.log('    nothing to merge')

  if (plan.ambiguous.length > 0) {
    console.log('\n==> left alone: a device without an os user whose name has several os users')
    console.log('    merging would hand one person the history of another, so pick by hand')
    for (const { device, osUsers } of plan.ambiguous) {
      console.log(`    ${describe(device)}: os users ${osUsers.join(', ')}`)
    }
  }

  if (!dryRun && plan.machines.length > 0) {
    console.log('\n==> merging')
    for (const m of await mergeDevices(db, plan)) {
      console.log(`    ${m.from} -> ${m.into}: ${m.days} day(s) and ${m.sessions} session(s) moved`)
    }
  }

  console.log('\n==> done')
  await client.close()
}

function machine(device: DeviceDocument): string {
  const name = device[FIELD.name] || '(unnamed)'
  const osUser = device[FIELD.osUser]
  return osUser ? `${name}/${osUser}` : name
}

function describe(device: DeviceDocument): string {
  const invite = device[FIELD.inviteId]?.toHexString() ?? 'no invite'
  const seen = device[FIELD.lastSeenAt].toISOString().slice(0, 10)
  return `${device[FIELD.id].toHexString()} (${machine(device)}, invite ${invite}, seen ${seen})`
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
