import Link from "next/link";

import type { PageInfo } from "@/lib/types";

/** Pagination renders previous/next links for a paged list. */
export default function Pagination({
  page,
  basePath,
  params = {},
}: {
  page: PageInfo;
  basePath: string;
  params?: Record<string, string | undefined>;
}) {
  if (page.total <= page.per_page) return null;

  const link = (n: number) => {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(params)) {
      if (value) query.set(key, value);
    }
    if (n > 1) query.set("page", String(n));
    const suffix = query.toString();
    return suffix ? `${basePath}?${suffix}` : basePath;
  };

  const lastPage = Math.max(1, Math.ceil(page.total / page.per_page));

  return (
    <nav className="pagination" aria-label="Pagination">
      {page.number > 1 ? (
        <Link href={link(page.number - 1)} className="btn btn-sm">
          ← Previous
        </Link>
      ) : (
        <span className="btn btn-sm" aria-disabled="true" style={{ opacity: 0.5 }}>
          ← Previous
        </span>
      )}
      <span className="small faint">
        Page {page.number} of {lastPage}
      </span>
      {page.has_next ? (
        <Link href={link(page.number + 1)} className="btn btn-sm">
          Next →
        </Link>
      ) : (
        <span className="btn btn-sm" aria-disabled="true" style={{ opacity: 0.5 }}>
          Next →
        </span>
      )}
    </nav>
  );
}
