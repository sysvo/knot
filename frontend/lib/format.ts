const minutes = 60;
const hours = minutes * 60;
const days = hours * 24;

/**
 * timeAgo renders a short relative timestamp.
 *
 * Output is expressed in whole units so that a server render and the hydration
 * a moment later agree; posts older than a month switch to an absolute date.
 */
export function timeAgo(unixSeconds: number, now = Math.floor(Date.now() / 1000)): string {
  if (!unixSeconds) return "—";
  const delta = now - unixSeconds;

  if (delta < 0) return "just now";
  if (delta < 60) return "just now";
  if (delta < hours) {
    const m = Math.floor(delta / minutes);
    return m === 1 ? "1 min ago" : `${m} min ago`;
  }
  if (delta < days) {
    const h = Math.floor(delta / hours);
    return h === 1 ? "1 hour ago" : `${h} hours ago`;
  }
  if (delta < days * 7) {
    const d = Math.floor(delta / days);
    return d === 1 ? "yesterday" : `${d} days ago`;
  }
  return formatDate(unixSeconds);
}

/** formatDate renders an absolute date, e.g. "12 Nov 2026". */
export function formatDate(unixSeconds: number): string {
  if (!unixSeconds) return "—";
  return new Date(unixSeconds * 1000).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

/** formatDateTime renders a date with the time of day. */
export function formatDateTime(unixSeconds: number): string {
  if (!unixSeconds) return "—";
  const d = new Date(unixSeconds * 1000);
  return `${formatDate(unixSeconds)}, ${d.toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
  })}`;
}

/** initials is the avatar fallback for a user without a picture. */
export function initials(name: string): string {
  const parts = name.replace(/[[\]]/g, "").trim().split(/[\s_-]+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return (parts[0] ?? "?").slice(0, 2).toUpperCase();
  return `${(parts[0] ?? "")[0] ?? ""}${
    (parts[parts.length - 1] ?? "")[0] ?? ""
  }`.toUpperCase();
}

/** plural picks the right word for a count. */
export function plural(n: number, one: string, many = `${one}s`): string {
  return n === 1 ? one : many;
}

/** formatBytes renders an upload size for humans. */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} kB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
