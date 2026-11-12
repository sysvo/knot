"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import type { Board, Thread } from "@/lib/types";

import MarkdownEditor from "./MarkdownEditor";

/** NewThreadForm creates a thread and navigates to it. */
export default function NewThreadForm({
  boards,
  initialBoard,
  signedIn,
  maxUploadBytes,
  registrationOpen,
}: {
  boards: Board[];
  initialBoard: string;
  signedIn: boolean;
  maxUploadBytes: number;
  registrationOpen: boolean;
}) {
  const router = useRouter();
  const [board, setBoard] = useState(initialBoard || boards[0]?.slug || "");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!signedIn) {
    return (
      <div className="empty">
        <h2>You need an account to start a thread</h2>
        <p className="small" style={{ marginBottom: "1rem" }}>
          {registrationOpen
            ? "It takes a username and a password."
            : "Registration is closed on this forum — ask an administrator for an account."}
        </p>
        <div className="row" style={{ justifyContent: "center" }}>
          <a href="/login" className="btn">
            Sign in
          </a>
          {registrationOpen && (
            <a href="/register" className="btn btn-primary">
              Create an account
            </a>
          )}
        </div>
      </div>
    );
  }

  async function submit() {
    if (title.trim().length < 3) {
      setError("Give the thread a title of at least 3 characters.");
      return;
    }
    if (!body.trim()) {
      setError("Write the first post.");
      return;
    }

    setBusy(true);
    setError(null);
    try {
      const result = await clientApi<{ thread: Thread }>(`/api/v1/boards/${board}/threads`, {
        method: "POST",
        body: { title, body },
      });
      router.push(`/t/${result.thread.id}`);
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not create the thread");
      setBusy(false);
    }
  }

  return (
    <div className="stack">
      {error && <div className="form-error">{error}</div>}

      <div className="field">
        <label htmlFor="thread-board">Board</label>
        <select
          id="thread-board"
          className="select"
          value={board}
          onChange={(event) => setBoard(event.target.value)}
        >
          {boards.map((option) => (
            <option key={option.slug} value={option.slug}>
              {option.name}
            </option>
          ))}
        </select>
      </div>

      <div className="field">
        <label htmlFor="thread-title">Title</label>
        <input
          id="thread-title"
          className="input"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          placeholder="What is this about?"
          maxLength={140}
          autoFocus
        />
        <span className="hint">{title.length}/140</span>
      </div>

      <div className="field">
        <label htmlFor="thread-body">First post</label>
        <MarkdownEditor
          value={body}
          onChange={setBody}
          placeholder="Write the opening post…"
          rows={12}
          maxUploadBytes={maxUploadBytes}
          id="thread-body"
        />
      </div>

      <div className="row">
        <button type="button" className="btn btn-primary" onClick={submit} disabled={busy}>
          {busy ? "Creating…" : "Create thread"}
        </button>
        <span className="tiny faint">
          Posting is public. Do not share personal information you would not publish.
        </span>
      </div>
    </div>
  );
}
