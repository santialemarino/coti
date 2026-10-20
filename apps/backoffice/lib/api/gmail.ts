import { listChannels } from '@/lib/api/channels';

export async function startGmailAuthorization(branchId: string, address: string): Promise<string> {
  const channels = await listChannels(branchId);
  const channel = channels.find(
    (item) => item.type === 'EMAIL' && item.identifier?.toLowerCase() === address.toLowerCase(),
  );
  if (!channel) throw new Error('The branch email channel is unavailable');
  const response = await fetch(
    `/api/channels/${encodeURIComponent(channel.id)}/gmail/authorization`,
    {
      method: 'POST',
      headers: { 'X-Branch-Id': branchId },
      cache: 'no-store',
    },
  );
  if (!response.ok) throw new Error('Gmail authorization is unavailable');
  const result = (await response.json()) as { authorization_url: string };
  const url = new URL(result.authorization_url);
  if (url.origin !== 'https://accounts.google.com' || url.username || url.password) {
    throw new Error('Invalid Google authorization URL');
  }
  return url.toString();
}
