import Link from "next/link";

import { getBoards, getMeta, getThreads } from "@/lib/server";
import ThreadList from "@/components/ThreadList";
import Timestamp from "@/components/Timestamp";

export default async function HomePage() {
  const [meta, boards, recent] = await Promise.all([
    getMeta(),
    getBoards(),
    getThreads({ perPage: 15 }),
  ]);

  return (
    <div className="stack" style={{ gap: "2rem" }}>
      <section className="page-head" style={{ marginBottom: 0 }}>
        <div>
          <h1>{meta.site_name}</h1>
          {meta.site_description && <p>{meta.site_description}</p>}
        </div>
        <div className="row">
          {meta.user ? (
            <Link href="/new" className="btn btn-primary">
              Start a thread
            </Link>
          ) : (
            <>
              <Link href="/login" className="btn">
                Sign in
              </Link>
              {meta.allow_registration && (
                <Link href="/register" className="btn btn-primary">
                  Join
                </Link>
              )}
            </>
          )}
        </div>
      </section>

      <section>
        <h2 style={{ fontSize: "0.78rem", textTransform: "uppercase", letterSpacing: "0.07em", color: "var(--text-faint)", marginBottom: "0.7rem" }}>
          Boards
        </h2>
        {boards.length === 0 ? (
          <div className="empty">
            <h2>No boards yet</h2>
            <p className="small">An administrator can create the first one in the admin panel.</p>
          </div>
        ) : (
          <div className="board-grid">
            {boards.map((board) => (
              <Link key={board.id} href={`/b/${board.slug}`} className="board-card">
                <h3>{board.name}</h3>
                {board.description && <p>{board.description}</p>}
                <div className="board-meta">
                  <span>{board.thread_count} threads</span>
                  <span>{board.post_count} posts</span>
                  {board.last_post_at > 0 && (
                    <span>
                      active <Timestamp at={board.last_post_at} />
                    </span>
                  )}
                </div>
              </Link>
            ))}
          </div>
        )}
      </section>

      <section>
        <div className="row-between" style={{ marginBottom: "0.7rem" }}>
          <h2 style={{ fontSize: "0.78rem", textTransform: "uppercase", letterSpacing: "0.07em", color: "var(--text-faint)" }}>
            Latest activity
          </h2>
          <Link href="/search" className="small">
            Search
          </Link>
        </div>
        <ThreadList threads={recent.threads} showBoard emptyMessage="Nothing posted yet" />
      </section>
    </div>
  );
}
