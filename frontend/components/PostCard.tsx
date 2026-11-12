"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import type { Post, Self } from "@/lib/types";
import { displayName } from "@/lib/types";

import Avatar from "./Avatar";
import Timestamp from "./Timestamp";

const REPORT_REASONS = ["Spam", "Harassment", "Off topic", "Illegal content", "Other"];

/**
 * PostCard renders one post with its actions.
 *
 * The post body arrives as sanitised HTML produced by the API when the post was
 * written, so it is inserted as-is; re-rendering Markdown in the browser would
 * mean shipping a second sanitiser and trusting both to agree.
 */
export default function PostCard({
  post,
  viewer,
  canModerate,
}: {
  post: Post;
  viewer: Self | null;
  canModerate: boolean;
}) {
  const router = useRouter();

  const [score, setScore] = useState(post.score);
  const [myVote, setMyVote] = useState(post.my_vote);
  const [body, setBody] = useState(post.body_md);
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reportOpen, setReportOpen] = useState(false);
  const [reported, setReported] = useState(false);

  const isAuthor = !!viewer && !!post.author && viewer.id === post.author.id;
  const mayEdit = !post.deleted && (isAuthor || canModerate);

  async function vote(value: number) {
    if (!viewer) {
      router.push("/login");
      return;
    }
    setError(null);
    try {
      // Clicking the arrow that is already active removes the vote.
      const next = myVote === value ? 0 : value;
      const result = await clientApi<{ score: number; my_vote: number }>(
        `/api/v1/posts/${post.id}/vote`,
        { method: "POST", body: { value: next } },
      );
      setScore(result.score);
      setMyVote(result.my_vote);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not save the vote");
    }
  }

  async function saveEdit() {
    setBusy(true);
    setError(null);
    try {
      await clientApi(`/api/v1/posts/${post.id}`, { method: "PATCH", body: { body } });
      setEditing(false);
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not save the edit");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!window.confirm("Delete this post? The thread keeps its place.")) return;
    setBusy(true);
    setError(null);
    try {
      await clientApi(`/api/v1/posts/${post.id}`, { method: "DELETE" });
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not delete the post");
    } finally {
      setBusy(false);
    }
  }

  async function submitReport(reason: string, details: string) {
    setBusy(true);
    setError(null);
    try {
      await clientApi(`/api/v1/posts/${post.id}/report`, {
        method: "POST",
        body: { reason, details },
      });
      setReported(true);
      setReportOpen(false);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not send the report");
    } finally {
      setBusy(false);
    }
  }

  return (
    <article className="post" id={`post-${post.id}`}>
      <div className="post-side">
        <Avatar user={post.author} size="lg" />
        <div>
          {post.author ? (
            <Link href={`/u/${post.author.username}`} className="post-author">
              {displayName(post.author)}
            </Link>
          ) : (
            <span className="post-author muted">[deleted]</span>
          )}
          <div className="row" style={{ gap: "0.3rem", marginTop: "0.15rem" }}>
            {post.author?.role === "admin" && <span className="badge badge-accent">Admin</span>}
            {post.author?.role === "moderator" && <span className="badge">Mod</span>}
            {post.author?.status === "banned" && <span className="badge badge-danger">Banned</span>}
          </div>
        </div>
      </div>

      <div className="post-body">
        <div className="post-head">
          <Timestamp at={post.created_at} />
          {post.edited_at && <span>· edited <Timestamp at={post.edited_at} /></span>}
          {post.pending && <span className="badge badge-warn">Awaiting review</span>}
          <a href={`#post-${post.id}`} className="faint tiny" aria-label="Link to this post">
            #
          </a>
        </div>

        {error && <div className="form-error" style={{ marginBottom: "0.6rem" }}>{error}</div>}

        {post.deleted ? (
          <p className="post-deleted">
            This post was removed{canModerate ? " — moderators can still see that it existed" : ""}.
          </p>
        ) : editing ? (
          <div className="stack-sm">
            <textarea
              className="textarea"
              value={body}
              onChange={(event) => setBody(event.target.value)}
              rows={10}
              aria-label="Edit post"
            />
            <div className="row">
              <button type="button" className="btn btn-primary btn-sm" onClick={saveEdit} disabled={busy}>
                {busy ? "Saving…" : "Save changes"}
              </button>
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={() => {
                  setEditing(false);
                  setBody(post.body_md);
                }}
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          // The HTML is sanitised by the API on write.
          <div className="prose" dangerouslySetInnerHTML={{ __html: post.body_html }} />
        )}

        {!post.deleted && !editing && (
          <div className="post-actions">
            <div className="vote">
              <button
                type="button"
                onClick={() => vote(1)}
                data-active={myVote === 1}
                aria-label="Upvote"
                title={viewer ? "Upvote" : "Sign in to vote"}
              >
                ▲
              </button>
              <span className="vote-score">{score}</span>
              <button
                type="button"
                onClick={() => vote(-1)}
                data-active={myVote === -1}
                aria-label="Downvote"
                title={viewer ? "Downvote" : "Sign in to vote"}
              >
                ▼
              </button>
            </div>

            {mayEdit && (
              <button type="button" className="link-button" onClick={() => setEditing(true)}>
                Edit
              </button>
            )}
            {mayEdit && (
              <button type="button" className="link-button danger" onClick={remove} disabled={busy}>
                Delete
              </button>
            )}
            {viewer && !isAuthor && !reported && (
              <button type="button" className="link-button" onClick={() => setReportOpen((v) => !v)}>
                Report
              </button>
            )}
            {reported && <span className="tiny faint">Reported, thank you</span>}
          </div>
        )}

        {reportOpen && (
          <ReportForm
            busy={busy}
            onCancel={() => setReportOpen(false)}
            onSubmit={submitReport}
          />
        )}
      </div>
    </article>
  );
}

function ReportForm({
  busy,
  onCancel,
  onSubmit,
}: {
  busy: boolean;
  onCancel: () => void;
  onSubmit: (reason: string, details: string) => void;
}) {
  const [reason, setReason] = useState(REPORT_REASONS[0] ?? "Spam");
  const [details, setDetails] = useState("");

  return (
    <div className="panel" style={{ marginTop: "0.75rem" }}>
      <div className="stack-sm">
        <div className="field">
          <label htmlFor="report-reason">Reason</label>
          <select
            id="report-reason"
            className="select"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
          >
            {REPORT_REASONS.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="report-details">Anything else? (optional)</label>
          <textarea
            id="report-details"
            className="textarea"
            style={{ minHeight: "4.5rem" }}
            value={details}
            onChange={(event) => setDetails(event.target.value)}
            maxLength={1000}
          />
        </div>
        <div className="row">
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={() => onSubmit(reason, details)}
            disabled={busy}
          >
            Send report
          </button>
          <button type="button" className="btn btn-ghost btn-sm" onClick={onCancel}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
