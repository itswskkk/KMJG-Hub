/**
 * Holds the File a sender picked for a Direct File Transfer until the
 * recipient accepts it. Per docs/PRD.md the bytes must not be uploaded
 * before acceptance, so the Client keeps the local File reference in memory
 * only; if it is lost (app restart), the sender re-selects the file and the
 * Server verifies its size matches the original request.
 */
const files = new Map<string, File>();

export function rememberTransferFile(transferId: string, file: File): void {
  files.set(transferId, file);
}

export function getTransferFile(transferId: string): File | undefined {
  return files.get(transferId);
}

export function forgetTransferFile(transferId: string): void {
  files.delete(transferId);
}
