"use client";

import Link from "next/link";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";

/** ForgotForm asks for a reset link. */
export default function ForgotForm() {
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      const result = await clientApi<{ message: string }>("/api/v1/auth/reset", {
        method: "POST",
        body: { email },
      });
      setSent(result.message);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not start the reset");
    } finally {
      setBusy(false);
    }
  }

  if (sent) {
    return (
      <div className="stack">
        <div className="form-success">{sent}</div>
        <p className="small muted" style={{ margin: 0 }}>
          The link works once and expires in an hour. Check your spam folder before assuming it
          did not arrive.
        </p>
        <p className="small" style={{ margin: 0 }}>
          <Link href="/login">Back to sign in</Link>
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
        <label htmlFor="forgot-email">Your e-mail address</label>
        <input
          id="forgot-email"
          className="input"
          type="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          autoComplete="email"
          autoFocus
          required
        />
        <span className="hint">We send a link that lets you choose a new password.</span>
      </div>

      <button type="submit" className="btn btn-primary btn-lg" disabled={busy}>
        {busy ? "Sending…" : "Send reset link"}
      </button>

      <p className="small muted" style={{ margin: 0 }}>
        <Link href="/login">Back to sign in</Link>
      </p>
    </form>
  );
}
