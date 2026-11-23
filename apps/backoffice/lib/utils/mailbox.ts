export function isTestMailbox(address: string): boolean {
  return address.trim().toLowerCase().endsWith('.test');
}
