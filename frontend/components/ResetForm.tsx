"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import type { Self } from "@/lib/types";

/**
 * ResetForm sets a new password from a reset link.
 *
 * The token is not consumed on load: the reader types a password first, and the
 * token is spent when they submit. A link that is consumed just by being opened
 * gets eaten by a mail scanner and then looks broken to the person it was sent to.
 */
export default function ResetForm({ token }: { token: string }) {
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);
  const mounted = useRef(false);

  useEffect(() => {
    mounted.current = true;
  }, []);

  if (!token) {
    return (
      <div className="stack">
        <div className="form-error">This link is missing its token.</div>
        <p className="small" style={{ margin: 0 }}>
          <Link href="/forgot">Request a new one</Link>
        </p>
      </div>
    );
  }

  async function submit() {
    if (password !== confirm) {
      setError("The two passwords do not match.");
      return;
    }
    if (password.length < 10) {
      setError("Use at least 10 characters.");
      return;
    }

    setBusy(true);
    setError(null);
    try {
      await clientApi<Self>("/api/v1/auth/reset/confirm", {
        method: "POST",
        body: { token, password },
      });
      setDone(true);
      // The API signs the reader in, so a refresh picks up the new session.
      router.refresh();
      window.setTimeout(() => router.push("/"), 900);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not set the password");
      setBusy(false);
    }
  }

  if (done) {
    return (
      <div className="stack">
        <div className="form-success">Password changed. Taking you to the forum…</div>
        <p className="tiny faint" style={{ margin: 0 }}>
          Every other device has been signed out.
        </p>
      </div>
    );
  }

  return (
    <form
      className="stack-sm"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      {error && <div className="form-error">{error}</div>}

      <div className="field">
        <label htmlFor="reset-password">New password</label>
        <input
          id="reset-password"
          className="input"
          type="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          autoComplete="new-password"
          minLength={10}
          autoFocus
          required
        />
        <span className="hint">At least 10 characters.</span>
      </div>

      <div className="field">
        <label htmlFor="reset-confirm">Repeat it</label>
        <input
          id="reset-confirm"
          className="input"
          type="password"
          value={confirm}
          onChange={(event) => setConfirm(event.target.value)}
          autoComplete="new-password"
          minLength={10}
          required
        />
      </div>

      <button type="submit" className="btn btn-primary btn-lg" disabled={busy}>
        {busy ? "Saving…" : "Set new password"}
      </button>

      <p className="small muted" style={{ margin: 0 }}>
        <Link href="/login">Back to sign in</Link>
      </p>
    </form>
  );
}
