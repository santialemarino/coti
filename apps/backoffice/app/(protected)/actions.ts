'use server';

import { redirect } from 'next/navigation';

import { LOGIN_ROUTE } from '@/config/routes';
import { endSession } from '@/lib/auth/session';

// Signing out revokes the session on the API as well as clearing the cookies, so
// the tokens the browser was holding stop working everywhere rather than only here.
export async function signOut(): Promise<void> {
  await endSession();
  redirect(LOGIN_ROUTE);
}
