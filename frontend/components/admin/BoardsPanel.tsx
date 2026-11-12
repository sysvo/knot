"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import type { Board } from "@/lib/types";

/**
 * BoardsPanel creates, renames and deletes boards.
 *
 * Inputs are uncontrolled with defaultValue and read through FormData: after a
 * save the server re-renders the page with fresh data, and uncontrolled inputs
 * pick that up without any state-syncing effect.
 */
export default function BoardsPanel({ boards }: { boards: Board[] }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function send(method: string, path: string, body: unknown) {
    setBusy(true);
    setError(null);
    try {
      await clientApi(path, { method, body });
      router.refresh();
      return true;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "the change was not saved");
      return false;
    } finally {
      setBusy(false);
    }
  }

  function read(form: HTMLFormElement) {
    const data = new FormData(form);
    return {
      slug: String(data.get("slug") ?? ""),
      name: String(data.get("name") ?? ""),
      description: String(data.get("description") ?? ""),
      position: Number.parseInt(String(data.get("position") ?? "0"), 10) || 0,
    };
  }

  return (
    <div className="stack">
      {error && <div className="form-error">{error}</div>}

      <div className="list">
        {boards.map((board) => (
          <form
            className="list-row"
            key={board.id}
            onSubmit={(event) => {
              event.preventDefault();
              void send("PATCH", `/api/v1/admin/boards/${board.id}`, read(event.currentTarget));
            }}
          >
            <div className="stack-sm" style={{ flex: 1, minWidth: 0 }}>
              <div className="row" style={{ gap: "0.5rem" }}>
                <input
                  name="name"
                  className="input"
                  defaultValue={board.name}
                  style={{ flex: "1 1 12rem", fontWeight: 600 }}
                  aria-label="Board name"
                  required
                />
                <input
                  name="slug"
                  className="input mono"
                  defaultValue={board.slug}
                  style={{ flex: "0 1 9rem" }}
                  aria-label="Slug"
                  required
                />
                <input
                  name="position"
                  className="input"
                  type="number"
                  defaultValue={board.position}
                  style={{ width: "5rem" }}
                  aria-label="Order"
                />
              </div>
              <input
                name="description"
                className="input"
                defaultValue={board.description}
                placeholder="Description"
                aria-label="Description"
              />
            </div>
            <div className="stack-sm" style={{ justifyContent: "center" }}>
              <button type="submit" className="btn btn-sm" disabled={busy}>
                Save
              </button>
              <button
                type="button"
                className="btn btn-danger btn-sm"
                disabled={busy}
                onClick={() => {
                  if (
                    window.confirm(
                      `Delete "${board.name}" with its ${board.thread_count} threads? This cannot be undone.`,
                    )
                  ) {
                    void send("DELETE", `/api/v1/admin/boards/${board.id}`, {});
                  }
                }}
              >
                Delete
              </button>
            </div>
          </form>
        ))}
      </div>

      <section className="card card-pad">
        <h2 style={{ fontSize: "1rem", marginBottom: "0.75rem" }}>New board</h2>
        <form
          className="stack-sm"
          onSubmit={(event) => {
            event.preventDefault();
            const form = event.currentTarget;
            void send("POST", "/api/v1/admin/boards", read(form)).then((ok) => {
              if (ok) form.reset();
            });
          }}
        >
          <div className="row" style={{ gap: "0.5rem" }}>
            <input
              name="name"
              className="input"
              placeholder="Name"
              style={{ flex: "1 1 12rem" }}
              aria-label="Board name"
              required
            />
            <input
              name="slug"
              className="input mono"
              placeholder="slug"
              style={{ flex: "0 1 9rem" }}
              aria-label="Slug"
              required
            />
            <input
              name="position"
              className="input"
              type="number"
              defaultValue={boards.length * 10 + 10}
              style={{ width: "5rem" }}
              aria-label="Order"
            />
          </div>
          <input name="description" className="input" placeholder="Description" aria-label="Description" />
          <div className="row">
            <button type="submit" className="btn btn-primary" disabled={busy}>
              Create board
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
