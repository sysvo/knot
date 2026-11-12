"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import { displayName, type PendingPost, type Thread } from "@/lib/types";
import Avatar from "../Avatar";
import Timestamp from "../Timestamp";

/**
 * PendingPanel is the review queue: content held because the author is new.
 *
 * Approving publishes an item and only then announces it, so a notification
 * never points at something the recipient cannot open.
 */
export default function PendingPanel({
  threads,
  posts,
}: {
  threads: Thread[];
  posts: PendingPost[];
}) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function act(action: "approve" | "reject", kind: "thread" | "post", id: number) {
    setBusy(true);
    setError(null);
    try {
      if (action === "approve") {
        await clientApi(`/api/v1/admin/${kind}s/${id}/approve`, { method: "POST" });
      } else {
        await clientApi(`/api/v1/${kind}s/${id}`, { method: "DELETE" });
      }
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : `could not ${action} that item`);
    } finally {
      setBusy(false);
    }
  }

  const total = threads.length + posts.length;

  return (
    <div className="stack">
      {error && <div className="form-error">{error}</div>}

      {total === 0 ? (
        <div className="empty">
          <h2>Nothing waiting</h2>
          <p className="small">
            New accounts are held for review only while that rule is enabled in settings.
          </p>
        </div>
      ) : (
        <div className="list">
          {threads.map((thread) => (
            <div className="list-row" key={`thread-${thread.id}`}>
              <Avatar user={thread.author} />
              <div className="list-row-body">
                <div className="row" style={{ gap: "0.5rem" }}>
                  <span className="badge badge-warn">New thread</span>
                  <span className="small faint">
                    {displayName(thread.author)} · <Timestamp at={thread.created_at} /> ·{" "}
                    {thread.board_name}
                  </span>
                </div>
                <Link href={`/t/${thread.id}`} className="list-row-title" style={{ marginTop: "0.3rem" }}>
                  {thread.title}
                </Link>
                {thread.excerpt && <p className="list-row-excerpt">{thread.excerpt}</p>}

                <div className="row" style={{ marginTop: "0.5rem" }}>
                  <button
                    type="button"
                    className="btn btn-primary btn-sm"
                    disabled={busy}
                    onClick={() => act("approve", "thread", thread.id)}
                  >
                    Approve and publish
                  </button>
                  <button
                    type="button"
                    className="btn btn-danger btn-sm"
                    disabled={busy}
                    onClick={() => act("reject", "thread", thread.id)}
                  >
                    Reject
                  </button>
                </div>
              </div>
            </div>
          ))}

          {posts.map((item) => (
            <div className="list-row" key={`post-${item.post.id}`}>
              <Avatar user={item.post.author} />
              <div className="list-row-body">
                <div className="row" style={{ gap: "0.5rem" }}>
                  <span className="badge badge-warn">New reply</span>
                  <span className="small faint">
                    {displayName(item.post.author)} · <Timestamp at={item.post.created_at} /> · in{" "}
                    <Link href={`/t/${item.post.thread_id}`}>{item.thread_title || "a thread"}</Link>
                  </span>
                </div>
                <p className="list-row-excerpt">{item.post.body_md.slice(0, 400)}</p>

                <div className="row" style={{ marginTop: "0.5rem" }}>
                  <button
                    type="button"
                    className="btn btn-primary btn-sm"
                    disabled={busy}
                    onClick={() => act("approve", "post", item.post.id)}
                  >
                    Approve and publish
                  </button>
                  <button
                    type="button"
                    className="btn btn-danger btn-sm"
                    disabled={busy}
                    onClick={() => act("reject", "post", item.post.id)}
                  >
                    Reject
                  </button>
                  <Link href={`/t/${item.post.thread_id}`} className="btn btn-ghost btn-sm">
                    Open thread
                  </Link>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
