"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import type { Thread } from "@/lib/types";

/**
 * ThreadModBar holds the thread-level actions: pin, lock, retitle and delete.
 * The API re-checks every permission, so this bar only decides what to show.
 */
export default function ThreadModBar({
  thread,
  viewerCanModerate,
  viewerIsAuthor,
}: {
  thread: Thread;
  viewerCanModerate: boolean;
  viewerIsAuthor: boolean;
}) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [editingTitle, setEditingTitle] = useState(false);
  const [title, setTitle] = useState(thread.title);

  if (thread.deleted) {
    return <div className="badge badge-danger">This thread was deleted</div>;
  }

  async function toggle(flag: "pin" | "lock") {
    setBusy(true);
    setError(null);
    try {
      await clientApi(`/api/v1/threads/${thread.id}/${flag}`, { method: "POST" });
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not update the thread");
    } finally {
      setBusy(false);
    }
  }

  async function saveTitle() {
    setBusy(true);
    setError(null);
    try {
      await clientApi(`/api/v1/threads/${thread.id}`, { method: "PATCH", body: { title } });
      setEditingTitle(false);
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not rename the thread");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!window.confirm("Delete this thread? It disappears from the board.")) return;
    setBusy(true);
    setError(null);
    try {
      await clientApi(`/api/v1/threads/${thread.id}`, { method: "DELETE" });
      router.push(`/b/${thread.board_slug}`);
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not delete the thread");
      setBusy(false);
    }
  }

  return (
    <div className="stack-sm">
      {error && <div className="form-error">{error}</div>}

      {editingTitle ? (
        <div className="row">
          <input
            className="input"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            maxLength={140}
            aria-label="Thread title"
            style={{ maxWidth: "28rem" }}
          />
          <button type="button" className="btn btn-primary btn-sm" onClick={saveTitle} disabled={busy}>
            Save
          </button>
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={() => {
              setEditingTitle(false);
              setTitle(thread.title);
            }}
          >
            Cancel
          </button>
        </div>
      ) : (
        <div className="row">
          {viewerCanModerate && (
            <button type="button" className="btn btn-sm" onClick={() => toggle("pin")} disabled={busy}>
              {thread.pinned ? "Unpin" : "Pin"}
            </button>
          )}
          {viewerCanModerate && (
            <button type="button" className="btn btn-sm" onClick={() => toggle("lock")} disabled={busy}>
              {thread.locked ? "Unlock" : "Lock"}
            </button>
          )}
          {(viewerIsAuthor || viewerCanModerate) && (
            <button type="button" className="btn btn-sm" onClick={() => setEditingTitle(true)}>
              Rename
            </button>
          )}
          {(viewerIsAuthor || viewerCanModerate) && (
            <button type="button" className="btn btn-danger btn-sm" onClick={remove} disabled={busy}>
              Delete thread
            </button>
          )}
        </div>
      )}
    </div>
  );
}
