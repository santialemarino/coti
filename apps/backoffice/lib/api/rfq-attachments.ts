export interface RfqAttachment {
  id: string;
  name: string | null;
  status: string;
  url: string;
}

export async function listRfqAttachments(
  rfqId: string,
  branchId: string,
): Promise<RfqAttachment[]> {
  const response = await fetch(`/api/rfqs/${encodeURIComponent(rfqId)}/attachments`, {
    cache: 'no-store',
    headers: { 'X-Branch-Id': branchId },
  });
  if (!response.ok) throw new Error('Unable to load RFQ attachments');
  const data = (await response.json()) as {
    attachments: Array<{
      id: string;
      original_filename: string | null;
      processing_status: string;
      url: string;
    }>;
  };
  return data.attachments.map((item) => ({
    id: item.id,
    name: item.original_filename,
    status: item.processing_status,
    url: item.url,
  }));
}
