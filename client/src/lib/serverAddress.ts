/**
 * Normalizes a user-entered KMJG Hub Server address into a full URL.
 * If no scheme is provided, https:// is assumed.
 */
export function normalizeServerAddress(input: string): string {
  const trimmed = input.trim();
  if (/^https?:\/\//i.test(trimmed)) {
    return trimmed;
  }
  return `https://${trimmed}`;
}

// Matches a hostname (e.g. "hub.example.com", "localhost") or an IPv4 address.
const HOSTNAME_PATTERN =
  /^([a-z0-9]([a-z0-9-]*[a-z0-9])?)(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$/i;

/**
 * Parses a user-entered KMJG Hub Server address, returning null when the
 * input cannot be interpreted as a valid server address.
 */
export function parseServerAddress(input: string): URL | null {
  const trimmed = input.trim();
  if (!trimmed || /\s/.test(trimmed)) {
    return null;
  }

  try {
    const url = new URL(normalizeServerAddress(trimmed));
    if (!HOSTNAME_PATTERN.test(url.hostname)) {
      return null;
    }
    return url;
  } catch {
    return null;
  }
}
