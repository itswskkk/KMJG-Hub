import { useConnectionStatus } from "../features/presence/PresenceProvider";
import "./OfflineBanner.css";

interface OfflineBannerProps {
  /** Extra text appended after the base offline message, e.g. naming what
   * is being shown instead ("showing cached projects"). */
  detail?: string;
  /** Show the offline message even though the real-time connection itself
   * is open — for a screen whose own HTTP fetch failed and fell back to
   * cached data (docs/PRD.md "Offline Behavior" applies just as much to a
   * failed fetch as to a fully offline connection). */
  forceShow?: boolean;
}

/**
 * Slim, reusable banner for docs/PRD.md "Offline Behavior". Shown whenever
 * the authenticated real-time connection is not open (or `forceShow` says a
 * screen is showing cached data for some other reason, e.g. a failed
 * fetch), with a brief "Syncing…" variant right after reconnecting while
 * screens refresh their data from the Server. Renders nothing while fully
 * connected, not syncing, and not forced, so mounting it is always safe.
 */
function OfflineBanner({ detail, forceShow }: OfflineBannerProps) {
  const { isOnline, isSyncing } = useConnectionStatus();

  if (isOnline && !forceShow) {
    if (!isSyncing) {
      return null;
    }
    return (
      <p className="offline-banner offline-banner--syncing" role="status">
        Syncing…
      </p>
    );
  }

  return (
    <p className="offline-banner" role="status">
      You&apos;re offline — showing last known data{detail ? ` (${detail})` : ""}.
    </p>
  );
}

export default OfflineBanner;
