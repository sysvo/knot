"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import { displayName, type User } from "@/lib/types";
import { formatDate } from "@/lib/format";
import Avatar from "../Avatar";

/** UsersPanel changes roles, bans accounts, and searches for people. */
export default function UsersPanel({ users, query }: { users: User[]; query: string }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function update(id: number, patch: Record<string, string>) {
    setBusy(true);
    setError(null);
    try {
      await clientApi(`/api/v1/admin/users/${id}`, { method: "PATCH", body: patch });
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "the change was not saved");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="stack">
      {error && <div className="form-error">{error}</div>}

      <form className="row" action="/admin" method="get" style={{ gap: "0.5rem" }}>
        <input type="hidden" name="tab" value="users" />
        <input
          className="input"
          type="search"
          name="q"
          defaultValue={query}
          placeholder="Search by username or e-mail…"
          style={{ flex: "1 1 16rem" }}
          aria-label="Search users"
        />
        <button type="submit" className="btn">
          Search
        </button>
      </form>

      {users.length === 0 ? (
        <div className="empty">
          <h2>No users found</h2>
        </div>
      ) : (
        <div className="list">
          {users.map((user) => (
            <form
              className="list-row"
              key={user.id}
              onSubmit={(event) => {
                event.preventDefault();
                const data = new FormData(event.currentTarget);
                void update(user.id, {
                  role: String(data.get("role") ?? user.role),
                  status: String(data.get("status") ?? user.status),
                });
              }}
            >
              <Avatar user={user} size="lg" />

              <div className="list-row-body">
                <div className="row" style={{ gap: "0.5rem" }}>
                  <Link href={`/u/${user.username}`} style={{ fontWeight: 600 }}>
                    {displayName(user)}
                  </Link>
                  <span className="tiny faint mono">@{user.username}</span>
                  {user.status === "banned" && <span className="badge badge-danger">Banned</span>}
                </div>
                <div className="list-row-meta">
                  <span>joined {formatDate(user.created_at)}</span>
                </div>

                <div className="row" style={{ gap: "0.5rem", marginTop: "0.5rem" }}>
                  <select
                    name="role"
                    className="select input-inline"
                    defaultValue={user.role}
                    aria-label={`Role for ${user.username}`}
                  >
                    <option value="member">member</option>
                    <option value="moderator">moderator</option>
                    <option value="admin">admin</option>
                  </select>
                  <select
                    name="status"
                    className="select input-inline"
                    defaultValue={user.status}
                    aria-label={`Status for ${user.username}`}
                  >
                    <option value="active">active</option>
                    <option value="banned">banned</option>
                  </select>
                  <button type="submit" className="btn btn-sm" disabled={busy}>
                    Save
                  </button>
                </div>
              </div>
            </form>
          ))}
        </div>
      )}
    </div>
  );
}
