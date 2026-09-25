/**
 * Thin IndexedDB-backed local cache used to satisfy docs/PRD.md "Offline
 * Behavior": when disconnected, the Client should still show "appropriate
 * locally cached information" such as previously loaded project
 * information, previous conversations, and last known member/Git status.
 *
 * This is a convenience layer only, per-viewer/per-browser-profile local
 * storage — never authoritative. Callers must always prefer a fresh HTTP
 * fetch when online, and only fall back to a cached value when offline or
 * the fetch fails (docs/VISION.md "Offline Support": "Server state always
 * wins"). Every operation here is wrapped so IndexedDB being unavailable
 * (private browsing, disabled, or any thrown error) never breaks the app —
 * callers see `undefined`/`[]` exactly as if nothing were cached yet.
 *
 * No external dependency: plain native `indexedDB` with a small Promise
 * wrapper, per the Phase 11 plan.
 */

const DB_NAME = "kmjg-hub-offline-cache";
const DB_VERSION = 1;

/** One object store per kind of cached data (Phase 11 plan scope):
 * - projects: Server Home's project list, and each Project's detail
 *   (members/role) — see cacheKey() for how list vs. detail keys are built.
 * - chat_messages: the latest page of Project Chat messages, per Project.
 * - tasks: a Project's task list (reserved for a future integration point).
 * - conversations: the Direct Message conversation list preview (reserved
 *   for a future integration point). */
export const CACHE_STORES = ["projects", "chat_messages", "tasks", "conversations"] as const;
export type CacheStore = (typeof CACHE_STORES)[number];

/** Builds a cache key namespaced by Server address (and optionally further
 * parts, e.g. a Project id) so cached data from one saved Server never
 * leaks into another's (docs/PRD.md "Multi-Server Client Support"). */
export function cacheKey(serverUrl: string, ...parts: string[]): string {
  return [serverUrl, ...parts].join("::");
}

let dbPromise: Promise<IDBDatabase | null> | null = null;

function openDb(): Promise<IDBDatabase | null> {
  if (dbPromise) {
    return dbPromise;
  }
  dbPromise = new Promise((resolve) => {
    if (typeof indexedDB === "undefined") {
      resolve(null);
      return;
    }
    try {
      const request = indexedDB.open(DB_NAME, DB_VERSION);
      request.onupgradeneeded = () => {
        const db = request.result;
        for (const store of CACHE_STORES) {
          if (!db.objectStoreNames.contains(store)) {
            db.createObjectStore(store);
          }
        }
      };
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => resolve(null);
      request.onblocked = () => resolve(null);
    } catch {
      resolve(null);
    }
  });
  return dbPromise;
}

/** Reads one cached value. Resolves to `undefined` whenever nothing is
 * cached under `key`, or IndexedDB is unavailable/fails for any reason. */
export async function cacheGet<T>(store: CacheStore, key: string): Promise<T | undefined> {
  try {
    const db = await openDb();
    if (!db) {
      return undefined;
    }
    return await new Promise<T | undefined>((resolve) => {
      try {
        const tx = db.transaction(store, "readonly");
        const req = tx.objectStore(store).get(key);
        req.onsuccess = () => resolve(req.result as T | undefined);
        req.onerror = () => resolve(undefined);
      } catch {
        resolve(undefined);
      }
    });
  } catch {
    return undefined;
  }
}

/** Writes one cached value, replacing whatever was under `key`. Never
 * throws — a failed write is silently dropped, since this cache is a
 * best-effort convenience rather than a requirement for the app to work. */
export async function cacheSet<T>(store: CacheStore, key: string, value: T): Promise<void> {
  try {
    const db = await openDb();
    if (!db) {
      return;
    }
    await new Promise<void>((resolve) => {
      try {
        const tx = db.transaction(store, "readwrite");
        tx.objectStore(store).put(value, key);
        tx.oncomplete = () => resolve();
        tx.onerror = () => resolve();
        tx.onabort = () => resolve();
      } catch {
        resolve();
      }
    });
  } catch {
    // Caching is best-effort only.
  }
}

/** Reads every value currently in `store`. Resolves to `[]` whenever the
 * store is empty, or IndexedDB is unavailable/fails for any reason. */
export async function cacheGetAll<T>(store: CacheStore): Promise<T[]> {
  try {
    const db = await openDb();
    if (!db) {
      return [];
    }
    return await new Promise<T[]>((resolve) => {
      try {
        const tx = db.transaction(store, "readonly");
        const req = tx.objectStore(store).getAll();
        req.onsuccess = () => resolve((req.result as T[] | undefined) ?? []);
        req.onerror = () => resolve([]);
      } catch {
        resolve([]);
      }
    });
  } catch {
    return [];
  }
}

/** Deletes one cached value. Never throws. */
export async function cacheDelete(store: CacheStore, key: string): Promise<void> {
  try {
    const db = await openDb();
    if (!db) {
      return;
    }
    await new Promise<void>((resolve) => {
      try {
        const tx = db.transaction(store, "readwrite");
        tx.objectStore(store).delete(key);
        tx.oncomplete = () => resolve();
        tx.onerror = () => resolve();
        tx.onabort = () => resolve();
      } catch {
        resolve();
      }
    });
  } catch {
    // Caching is best-effort only.
  }
}

/** Test-only: resets the cached open-database handle so each test starts
 * from a clean slate against a fresh fake IndexedDB instance. */
export function _resetForTests(): void {
  dbPromise = null;
}
