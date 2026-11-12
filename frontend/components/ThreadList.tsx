import Link from "next/link";

import type { Thread } from "@/lib/types";
import { displayName } from "@/lib/types";
import { plural } from "@/lib/format";

import Avatar from "./Avatar";
import Timestamp from "./Timestamp";

/** ThreadList renders a list of threads. */
export default function ThreadList({
  threads,
  showBoard = false,
  emptyMessage = "Nothing here yet.",
}: {
  threads: Thread[];
  showBoard?: boolean;
  emptyMessage?: string;
}) {
  if (threads.length === 0) {
    return (
      <div className="empty">
        <h2>{emptyMessage}</h2>
        <p className="small">Start a thread and it will show up here.</p>
      </div>
    );
  }

  return (
    <div className="list">
      {threads.map((thread) => (
        <article className="list-row" key={thread.id}>
          <Avatar user={thread.last_author ?? thread.author} />

          <div className="list-row-body">
            <Link href={`/t/${thread.id}`} className="list-row-title">
              {thread.pinned && <span className="tag">Pinned</span>}
              {thread.locked && <span className="tag" title="Locked">Locked</span>}
              {thread.deleted && <span className="tag">Deleted</span>}
              {thread.title}
            </Link>

            {thread.excerpt && <p className="list-row-excerpt">{thread.excerpt}</p>}

            <div className="list-row-meta">
              {showBoard && (
                <span>
                  in{" "}
                  <Link href={`/b/${thread.board_slug}`} className="muted">
                    {thread.board_name}
                  </Link>
                </span>
              )}
              <span>
                started by{" "}
                <Link href={`/u/${thread.author?.username ?? ""}`} className="muted">
                  {displayName(thread.author)}
                </Link>
              </span>
              <span>
                {thread.post_count} {plural(thread.post_count, "post")}
              </span>
              <span>
                last reply <Timestamp at={thread.last_post_at} />
              </span>
            </div>
          </div>
        </article>
      ))}
    </div>
  );
}
