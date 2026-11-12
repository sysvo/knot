"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import type { Self } from "@/lib/types";

/** RegisterForm creates an account and signs the reader in. */
export default function RegisterForm({
  providers,
  requireVerification,
  note,
}: {
  providers: string[];
  requireVerification: boolean;
  note: string;
}) {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      await clientApi<Self>("/api/v1/auth/register", {
        method: "POST",
        body: { username, email, password },
      });
      router.push("/");
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not create the account");
      setBusy(false);
    }
  }

  return (
    <div className="stack">
      {note && <div className="panel small">{note}</div>}
      {error && <div className="form-error">{error}</div>}

      <form
        className="stack-sm"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <div className="field">
          <label htmlFor="register-username">Username</label>
          <input
            id="register-username"
            className="input"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            autoComplete="username"
            autoFocus
            required
            minLength={3}
            maxLength={32}
            pattern="[A-Za-z0-9][A-Za-z0-9_-]{1,30}[A-Za-z0-9]"
          />
          <span className="hint">3–32 characters. Letters, numbers, dash or underscore.</span>
        </div>

        <div className="field">
          <label htmlFor="register-email">E-mail (optional)</label>
          <input
            id="register-email"
            className="input"
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            autoComplete="email"
          />
          <span className="hint">
            {requireVerification
              ? "This forum requires a confirmed address before you can post."
              : "Used for password recovery and notifications. Never shown publicly."}
          </span>
        </div>

        <div className="field">
          <label htmlFor="register-password">Password</label>
          <input
            id="register-password"
            className="input"
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            autoComplete="new-password"
            required
            minLength={10}
          />
          <span className="hint">At least 10 characters. A passphrase is easier to remember.</span>
        </div>

        <button type="submit" className="btn btn-primary btn-lg" disabled={busy}>
          {busy ? "Creating…" : "Create account"}
        </button>
      </form>

      {providers.length > 0 && (
        <>
          <div className="divider">or continue with</div>
          <div className="oauth-buttons">
            {providers.map((provider) => (
              <a key={provider} className="btn" href={`/api/v1/auth/oauth/${provider}/start`}>
                {provider === "github" ? "GitHub" : provider === "discord" ? "Discord" : provider}
              </a>
            ))}
          </div>
        </>
      )}

      <p className="small muted" style={{ margin: 0 }}>
        Already have an account? <Link href="/login">Sign in</Link>.
      </p>
    </div>
  );
}
