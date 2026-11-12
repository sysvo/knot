"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import type { Post } from "@/lib/types";

import MarkdownEditor from "./MarkdownEditor";

/** ReplyForm posts a reply at the end of a thread. */
export default function ReplyForm({
  threadId,
  signedIn,
  locked,
  canPostInLocked,
  maxUploadBytes,
}: {
  threadId: number;
  signedIn: boolean;
  locked: boolean;
  canPostInLocked: boolean;
  maxUploadBytes: number;
}) {
  const router = useRouter();
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [posted, setPosted] = useState(false);

  if (!signedIn) {
    return (
      <div className="panel center">
        <p style={{ marginBottom: "0.6rem" }}>
          <Link href="/login">Sign in</Link> to join the conversation.
        </p>
      </div>
    );
  }

  if (locked && !canPostInLocked) {
    return (
      <div className="panel center muted small">
        This thread is locked. Moderators can still post.
      </div>
    );
  }

  async function submit() {
    if (!body.trim()) {
      setError("Write something first.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const result = await clientApi<{ post: Post }>(`/api/v1/threads/${threadId}/posts`, {
        method: "POST",
        body: { body },
      });
      setBody("");
      setPosted(true);
      router.refresh();
      // Bring the new reply into view once the page has re-rendered.
      window.setTimeout(() => {
        document.getElementById(`post-${result.post.id}`)?.scrollIntoView({ behavior: "smooth" });
      }, 400);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not post the reply");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="stack-sm">
      <h2 style={{ fontSize: "1rem" }}>Reply</h2>
      {error && <div className="form-error">{error}</div>}
      {posted && !error && <div className="form-success">Posted.</div>}
      <MarkdownEditor
        value={body}
        onChange={setBody}
        placeholder="Write a reply…"
        rows={7}
        maxUploadBytes={maxUploadBytes}
        id="reply-body"
        onSubmit={submit}
      />
      <div className="row">
        <button type="button" className="btn btn-primary" onClick={submit} disabled={busy}>
          {busy ? "Posting…" : "Post reply"}
        </button>
        <span className="tiny faint">
          Ctrl+Enter to post
        </span>
      </div>
    </div>
  );
}
