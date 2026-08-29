/**
 * The credentials the API authenticates with, read out of DATABASE_URL, and the statement that
 * gives the role its password. The migration chain creates coti_app without one, so a fresh
 * database refuses every connection as it until this has run, or a deployment has set its own.
 */

export function appRoleCredentials(databaseUrl) {
  const url = new URL(databaseUrl);
  const role = decodeURIComponent(url.username);
  const password = decodeURIComponent(url.password);
  if (!role || !password) {
    throw new Error('DATABASE_URL must carry the app role and its password.');
  }
  return { role, password };
}

// ALTER ROLE accepts no bind parameters, so the driver's own escaping stands in for them.
export async function setAppRolePassword(client, { role, password }) {
  await client.query(
    `ALTER ROLE ${client.escapeIdentifier(role)} PASSWORD ${client.escapeLiteral(password)}`,
  );
}
