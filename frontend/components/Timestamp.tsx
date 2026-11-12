import { formatDateTime, timeAgo } from "@/lib/format";

/**
 * Timestamp renders a relative time with the exact moment in the tooltip.
 *
 * suppressHydrationWarning is deliberate: the server renders "5 min ago" and the
 * browser may hydrate a second later, which can cross a rounding boundary.
 */
export default function Timestamp({
  at,
  className,
}: {
  at: number;
  className?: string;
}) {
  if (!at) return <span className={className}>—</span>;
  return (
    <time
      dateTime={new Date(at * 1000).toISOString()}
      title={formatDateTime(at)}
      className={className}
      suppressHydrationWarning
    >
      {timeAgo(at)}
    </time>
  );
}
