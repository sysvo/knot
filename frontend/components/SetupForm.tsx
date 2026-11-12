"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import type { Self } from "@/lib/types";

/**
 * SetupForm creates the first administrator and names the forum.
 *
 * The mail relay fields are optional on purpose: a forum that cannot send mail is
 * still a forum, and asking for an SMTP password before the person has seen the
 * thing running is a good way to lose them at step one.
 */
export default function SetupForm() {
  const router = useRouter();
  const [siteName, setSiteName] = useState("");
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [smtpHost, setSmtpHost] = useState("");
  const [smtpPort, setSmtpPort] = useState("587");
  const [smtpFrom, setSmtpFrom] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      await clientApi<Self>("/api/v1/setup", {
        method: "POST",
        body: {
          site_name: siteName,
          username,
          email,
          password,
          smtp_host: smtpHost,
          smtp_port: smtpHost ? smtpPort : "",
          smtp_from: smtpHost ? smtpFrom : "",
        },
      });
      router.push("/");
      router.refresh();
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError("This forum already has an account, so setup is closed. Sign in instead.");
      } else {
        setError(err instanceof ApiError ? err.message : "could not finish setup");
      }
      setBusy(false);
    }
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
        <label htmlFor="setup-site">Forum name</label>
        <input
          id="setup-site"
          className="input"
          value={siteName}
          onChange={(event) => setSiteName(event.target.value)}
          placeholder="knot"
          maxLength={40}
          autoFocus
        />
        <span className="hint">Shown in the header, the page title and outgoing mail.</span>
      </div>

      <div className="field">
        <label htmlFor="setup-username">Administrator username</label>
        <input
          id="setup-username"
          className="input"
          value={username}
          onChange={(event) => setUsername(event.target.value)}
          autoComplete="username"
          required
          minLength={3}
          maxLength={32}
          pattern="[A-Za-z0-9][A-Za-z0-9_\-]{1,30}[A-Za-z0-9]"
        />
        <span className="hint">3–32 characters. Letters, numbers, dash or underscore.</span>
      </div>

      <div className="field">
        <label htmlFor="setup-email">E-mail address</label>
        <input
          id="setup-email"
          className="input"
          type="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          autoComplete="email"
        />
        <span className="hint">
          Optional, but without it password resets and magic links cannot reach you.
        </span>
      </div>

      <div className="field">
        <label htmlFor="setup-password">Password</label>
        <input
          id="setup-password"
          className="input"
          type="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          autoComplete="new-password"
          required
          minLength={10}
        />
        <span className="hint">At least 10 characters.</span>
      </div>

      <fieldset className="fieldset">
        <legend>Mail relay (optional)</legend>
        <p className="hint" style={{ marginBottom: "0.75rem" }}>
          Needed for verification mail and password resets. You can add it later under Admin →
          Settings.
        </p>
        <div className="stack-sm">
          <div className="field">
            <label htmlFor="setup-smtp-host">SMTP host</label>
            <input
              id="setup-smtp-host"
              className="input"
              value={smtpHost}
              onChange={(event) => setSmtpHost(event.target.value)}
              placeholder="smtp.example.com"
              autoComplete="off"
            />
          </div>
          <div className="grid-2">
            <div className="field">
              <label htmlFor="setup-smtp-port">Port</label>
              <input
                id="setup-smtp-port"
                className="input"
                value={smtpPort}
                onChange={(event) => setSmtpPort(event.target.value)}
                inputMode="numeric"
                placeholder="587"
              />
            </div>
            <div className="field">
              <label htmlFor="setup-smtp-from">Sender address</label>
              <input
                id="setup-smtp-from"
                className="input"
                type="email"
                value={smtpFrom}
                onChange={(event) => setSmtpFrom(event.target.value)}
                placeholder="forum@example.com"
              />
            </div>
          </div>
        </div>
      </fieldset>

      <div className="row" style={{ marginTop: "0.5rem" }}>
        <button className="btn btn-primary" type="submit" disabled={busy}>
          {busy ? "Setting up…" : "Create forum"}
        </button>
      </div>
    </form>
  );
}
