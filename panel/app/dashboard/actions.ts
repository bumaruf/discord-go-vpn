'use server'

import { signOut } from '@/auth'

/** A Server Action, so signing out is a POST: a GET that signs out allows logout CSRF. */
export async function signOutAdmin() {
  await signOut({ redirectTo: '/login' })
}
