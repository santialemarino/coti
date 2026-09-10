/*
 * The intake channels a branch has open. A file-borne order has to name the channel it arrived
 * through, so the creator offers this list; the data comes through the /api/channels BFF proxy,
 * which forwards the caller's auth and branch.
 */

export type ChannelType = 'WHATSAPP' | 'EMAIL' | 'WEBAPP' | 'MANUAL_ENTRY';

export interface Channel {
  id: string;
  type: ChannelType;
  identifier: string | null;
}

interface ChannelListRaw {
  items: Array<{ id: string; type: ChannelType; identifier: string | null; is_active: boolean }>;
}

export async function listChannels(branchId: string | null): Promise<Channel[]> {
  const response = await fetch('/api/channels', {
    cache: 'no-store',
    headers: branchId ? { 'X-Branch-Id': branchId } : undefined,
  });

  if (!response.ok) return [];

  const data = (await response.json()) as ChannelListRaw;
  return data.items
    .filter((item) => item.is_active)
    .map((item) => ({ id: item.id, type: item.type, identifier: item.identifier }));
}
