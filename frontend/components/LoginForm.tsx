"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import type { Self } from "@/lib/types";

type Mode = "password" | "magic";

/** LoginForm signs in with a password, a magic link, or an OAuth provider. */
export default function LoginForm({
  providers,
  mailEnabled,
  allowRegistration,
  initialError,
  next,
}: {
  providers: string[];
  mailEnabled: boolean;
  allowRegistration: boolean;
  initialError?: string;
  next?: string;
}) {
  const router = useRouter();
  const [mode, setMode] = useState<Mode>("password");
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(initialError ?? null);
  const [notice, setNotice] = useState<string | null>(null);

  const target = next && next.startsWith("/") && !next.startsWith("//") ? next : "/";

  async function submitPassword() {
    setBusy(true);
    setError(null);
    try {
      await clientApi<Self>("/api/v1/auth/login", {
        method: "POST",
        body: { login, password },
      });
      router.push(target);
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not sign in");
      setBusy(false);
    }
  }

  async function submitMagic() {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const result = await clientApi<{ message: string }>("/api/v1/auth/magic", {
        method: "POST",
        body: { email },
      });
      setNotice(result.message);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not send the link");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="stack">
      {error && <div className="form-error">{error}</div>}
      {notice && <div className="form-success">{notice}</div>}

      {mode === "password" ? (
        <form
          className="stack-sm"
          onSubmit={(event) => {
            event.preventDefault();
            void submitPassword();
          }}
        >
          <div className="field">
            <label htmlFor="login-name">Username or e-mail</label>
            <input
              id="login-name"
              className="input"
              value={login}
              onChange={(event) => setLogin(event.target.value)}
              autoComplete="username"
              autoFocus
              required
            />
          </div>
          <div className="field">
            <label htmlFor="login-password">Password</label>
            <input
              id="login-password"
              className="input"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="current-password"
              required
            />
          </div>
          <button type="submit" className="btn btn-primary btn-lg" disabled={busy}>
            {busy ? "Signing in…" : "Sign in"}
          </button>
        </form>
      ) : (
        <form
          className="stack-sm"
          onSubmit={(event) => {
            event.preventDefault();
            void submitMagic();
          }}
        >
          <div className="field">
            <label htmlFor="login-email">E-mail</label>
            <input
              id="login-email"
              className="input"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              autoComplete="email"
              autoFocus
              required
            />
            <span className="hint">We send a link that signs you in once.</span>
          </div>
          <button type="submit" className="btn btn-primary btn-lg" disabled={busy}>
            {busy ? "Sending…" : "Email me a link"}
          </button>
        </form>
      )}

      {mailEnabled && (
        <button
          type="button"
          className="link-button"
          onClick={() => {
            setMode(mode === "password" ? "magic" : "password");
            setError(null);
            setNotice(null);
          }}
        >
          {mode === "password" ? "Sign in with an e-mail link instead" : "Use a password instead"}
        </button>
      )}

      {providers.length > 0 && (
        <>
          <div className="divider">or continue with</div>
          <div className="oauth-buttons">
            {providers.map((provider) => (
              <a
                key={provider}
                className="btn"
                href={`/api/v1/auth/oauth/${provider}/start?redirect=${encodeURIComponent(target)}`}
              >
                {provider === "github" ? "GitHub" : provider === "discord" ? "Discord" : provider}
              </a>
            ))}
          </div>
        </>
      )}

      {allowRegistration && (
        <p className="small muted" style={{ margin: 0 }}>
          No account yet? <Link href="/register">Create one</Link>.
        </p>
      )}
    </div>
  );
}
