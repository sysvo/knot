import type { Metadata } from "next";
import Link from "next/link";

import { getBoards, search } from "@/lib/server";
import Timestamp from "@/components/Timestamp";

export const metadata: Metadata = { title: "Search" };

export default async function SearchPage({
  searchParams,
}: {
  searchParams: Promise<{ q?: string; board?: string }>;
}) {
  const { q = "", board = "" } = await searchParams;
  const query = q.trim();
  const [boards, results] = await Promise.all([getBoards(), query ? search(query, board) : []]);

  return (
    <div className="stack">
      <div className="page-head">
        <div>
          <h1>Search</h1>
          <p>Full-text search across every thread title and post.</p>
        </div>
      </div>

      <form className="row" action="/search" method="get" style={{ gap: "0.5rem" }}>
        <input
          className="input"
          type="search"
          name="q"
          defaultValue={query}
          placeholder="Search posts…"
          aria-label="Search posts"
          autoFocus
          style={{ flex: "1 1 18rem" }}
        />
        <select className="select input-inline" name="board" defaultValue={board} aria-label="Board">
          <option value="">All boards</option>
          {boards.map((option) => (
            <option key={option.slug} value={option.slug}>
              {option.name}
            </option>
          ))}
        </select>
        <button type="submit" className="btn btn-primary">
          Search
        </button>
      </form>

      {query && (
        <p className="small muted" style={{ margin: 0 }}>
          {results.length === 0
            ? "No results."
            : `${results.length} ${results.length === 1 ? "result" : "results"} for “${query}”.`}
        </p>
      )}

      {results.length > 0 && (
        <div className="list">
          {results.map((hit) => (
            <div className="list-row" key={`${hit.post_id}`}>
              <div className="list-row-body">
                <Link href={`/t/${hit.thread_id}#post-${hit.post_id}`} className="list-row-title">
                  {hit.title}
                </Link>
                <p
                  className="list-row-excerpt snippet"
                  // Safe to inject: the API escapes the indexed text and adds the
                  // <mark> tags itself, so the only markup here is its own. Post
                  // text is never trusted as HTML.
                  dangerouslySetInnerHTML={{ __html: hit.snippet }}
                />
                <div className="list-row-meta">
                  <span>
                    in{" "}
                    <Link href={`/b/${hit.board_slug}`} className="muted">
                      {hit.board_name}
                    </Link>
                  </span>
                  {hit.author && <span>by {hit.author}</span>}
                  <span>
                    last reply <Timestamp at={hit.last_post_at} />
                  </span>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
