// Prints ADMIN_PASSWORD_HASH in base64 (see `decodeHash` in lib/admin.ts):
//   npm run hash-password -- "password"
import { hashPassword } from '../lib/password'

async function main() {
  const password = process.argv[2]
  if (!password) {
    console.error('usage: npm run hash-password -- "password"')
    process.exit(1)
  }
  if (password.length < 10) {
    console.error('the password needs at least 10 characters')
    process.exit(1)
  }

  const phc = await hashPassword(password)
  console.log(`\nADMIN_PASSWORD_HASH=${Buffer.from(phc, 'utf8').toString('base64')}\n`)
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
