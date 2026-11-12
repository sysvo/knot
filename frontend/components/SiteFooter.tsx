import Link from "next/link";

/** SiteFooter is the quiet strip below the content. */
export default function SiteFooter({
  siteName,
  version,
}: {
  siteName: string;
  version: string;
}) {
  return (
    <footer className="site-footer">
      <div className="container row-between">
        <span>
          {siteName} runs on{" "}
          <a href="https://github.com/sysvo/knot" rel="noreferrer">
            knot
          </a>
          , free software under the AGPL-3.0.
        </span>
        <span className="row" style={{ gap: "0.75rem" }}>
          <Link href="/search">Search</Link>
          <span className="faint mono tiny">{version}</span>
        </span>
      </div>
    </footer>
  );
}
