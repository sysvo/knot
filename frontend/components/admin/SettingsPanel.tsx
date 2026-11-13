"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import { formatDateTime } from "@/lib/format";
import type { MailQueue } from "@/lib/types";

type Field = {
  key: string;
  label: string;
  hint?: string;
  type?: "text" | "textarea" | "checkbox" | "number" | "password";
  placeholder?: string;
};

const SITE_FIELDS: Field[] = [
  { key: "site_name", label: "Site name", hint: "Shown in the header and page titles." },
  { key: "site_description", label: "Site description", type: "textarea", hint: "Used as the meta description." },
  { key: "allow_registration", label: "Allow registration", type: "checkbox", hint: "Turn off to make the forum invite-only." },
  {
    key: "require_email_verification",
    label: "Require confirmed e-mail",
    type: "checkbox",
    hint: "Readers must confirm their address before posting. Has no effect until mail works.",
  },
  {
    key: "registration_note",
    label: "Note on the registration page",
    type: "textarea",
    hint: "For example the rules, or how to request an invite.",
  },
];

const SPAM_FIELDS: Field[] = [
  {
    key: "spam_links_require_posts",
    label: "Posts before links are allowed",
    type: "number",
    hint: "A new account cannot post links until it has this many published posts.",
  },
  {
    key: "spam_links_require_days",
    label: "Days before links are allowed",
    type: "number",
    hint: "…and has been a member for this many days. Both conditions must be met. 0 turns the rule off.",
  },
  {
    key: "spam_new_posts_per_day",
    label: "Daily post limit for new accounts",
    type: "number",
    hint: "Applies only until an account qualifies to post links. 0 means no limit.",
  },
  {
    key: "spam_approve_first_posts",
    label: "Hold the first posts for review",
    type: "number",
    hint: "How many of a new account's first posts wait for a moderator. 0 publishes immediately — only enable this if you check the queue daily, or new members will think the forum is broken.",
  },
  {
    key: "spam_blocked_email_domains",
    label: "Blocked e-mail domains",
    type: "textarea",
    hint: "Registration is refused for these domains, one per line. Subdomains are included. Leave empty unless you are being hit.",
  },
];

const MAIL_FIELDS: Field[] = [
  {
    key: "smtp_host",
    label: "SMTP host",
    placeholder: "smtp.example.com",
    hint: "Leave empty to disable e-mail. Password resets and magic links then rely on the command line.",
  },
  {
    key: "smtp_port",
    label: "Port",
    type: "number",
    hint: "587 with STARTTLS. Port 465 (implicit TLS) is not supported.",
  },
  { key: "smtp_user", label: "Username", hint: "Leave empty if the relay needs no authentication." },
  {
    key: "smtp_pass",
    label: "Password",
    type: "password",
    hint: "Leave blank to keep the stored password. It is kept in the database, so treat the database file as the secret it is.",
  },
  {
    key: "smtp_from",
    label: "Sender address",
    placeholder: "forum@example.com",
    hint: "Must normally be an address your relay is allowed to send as.",
  },
];

const ALL_FIELDS = [...SITE_FIELDS, ...SPAM_FIELDS, ...MAIL_FIELDS];

/** SettingsPanel edits site, spam and mail settings, and shows the mail queue. */
export default function SettingsPanel({
  settings,
  mail,
}: {
  settings: Record<string, string>;
  mail: MailQueue;
}) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);

  const [testTo, setTestTo] = useState(settings.smtp_from ?? "");
  const [testBusy, setTestBusy] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; text: string } | null>(null);

  // Message bodies are fetched one at a time: a queued magic link is a working
  // credential, so reading one is a deliberate action rather than something the
  // page loads on its own. The API writes it to the audit log.
  const [bodies, setBodies] = useState<Record<number, string>>({});

  const passwordSet = settings.smtp_password_set === "true";

  async function save(form: HTMLFormElement) {
    setBusy(true);
    setMessage(null);

    const data = new FormData(form);
    const payload: Record<string, string> = {};
    for (const field of ALL_FIELDS) {
      payload[field.key] =
        field.type === "checkbox"
          ? data.get(field.key)
            ? "true"
            : "false"
          : String(data.get(field.key) ?? "");
    }

    try {
      await clientApi("/api/v1/admin/settings", { method: "PATCH", body: { settings: payload } });
      setMessage({ ok: true, text: "Settings saved." });
      router.refresh();
    } catch (err) {
      setMessage({
        ok: false,
        text: err instanceof ApiError ? err.message : "the settings were not saved",
      });
    } finally {
      setBusy(false);
    }
  }

  async function sendTest() {
    setTestBusy(true);
    setTestResult(null);
    try {
      await clientApi("/api/v1/admin/mail/test", { method: "POST", body: { to: testTo } });
      setTestResult({ ok: true, text: `Sent to ${testTo}. Check the inbox.` });
    } catch (err) {
      // The API returns the relay's own words here; that message is the point.
      setTestResult({
        ok: false,
        text: err instanceof ApiError ? err.message : "the test message could not be sent",
      });
    } finally {
      setTestBusy(false);
    }
  }

  async function revealBody(id: number) {
    // Toggle: a second click hides it again.
    if (bodies[id] !== undefined) {
      setBodies((prev) => {
        const next = { ...prev };
        delete next[id];
        return next;
      });
      return;
    }
    try {
      const data = await clientApi<{ message: { body: string } }>(`/api/v1/admin/mail/${id}`);
      setBodies((prev) => ({ ...prev, [id]: data.message.body }));
    } catch (err) {
      setBodies((prev) => ({
        ...prev,
        [id]: err instanceof ApiError ? err.message : "could not read the message",
      }));
    }
  }

  async function act(id: number, action: "retry" | "delete") {
    try {
      if (action === "retry") {
        await clientApi(`/api/v1/admin/mail/${id}/retry`, { method: "POST" });
      } else {
        await clientApi(`/api/v1/admin/mail/${id}`, { method: "DELETE" });
      }
      router.refresh();
    } catch (err) {
      setTestResult({
        ok: false,
        text: err instanceof ApiError ? err.message : "the queue was not updated",
      });
    }
  }

  return (
    <div className="stack" style={{ maxWidth: "44rem" }}>
      <form
        className="card card-pad stack"
        onSubmit={(event) => {
          event.preventDefault();
          void save(event.currentTarget);
        }}
      >
        <fieldset style={{ border: 0, padding: 0, margin: 0 }} className="stack-sm">
          <legend className="setting-legend">Site</legend>
          {SITE_FIELDS.map((field) => (
            <FieldRow key={field.key} field={field} value={settings[field.key] ?? ""} />
          ))}
        </fieldset>

        <fieldset style={{ border: 0, padding: 0, margin: 0 }} className="stack-sm">
          <legend className="setting-legend">Spam protection</legend>
          <p className="tiny faint" style={{ margin: "0 0 0.4rem" }}>
            Moderation and administration accounts are never restricted by these rules.
          </p>
          {SPAM_FIELDS.map((field) => (
            <FieldRow key={field.key} field={field} value={settings[field.key] ?? ""} />
          ))}
        </fieldset>

        <fieldset style={{ border: 0, padding: 0, margin: 0 }} className="stack-sm">
          <legend className="setting-legend">E-mail</legend>
          <p className="tiny faint" style={{ margin: "0 0 0.4rem" }}>
            {mail.configured
              ? "Mail is configured. Messages are queued and delivered in the background."
              : "No mail server is set. Magic links and password resets are unavailable until one is, and anything queued waits below."}
          </p>
          {MAIL_FIELDS.map((field) => (
            <FieldRow
              key={field.key}
              field={field}
              value={field.key === "smtp_pass" && passwordSet ? "" : (settings[field.key] ?? "")}
            />
          ))}
        </fieldset>

        {/* The form is taller than a screen, so the confirmation belongs beside
            the button rather than at the top where nobody will scroll back to. */}
        {message && <div className={message.ok ? "form-success" : "form-error"}>{message.text}</div>}

        <div className="row">
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {busy ? "Saving…" : "Save settings"}
          </button>
        </div>
      </form>

      <section className="card card-pad stack-sm">
        <h2 style={{ fontSize: "1.05rem" }}>Send a test message</h2>
        <p className="tiny faint" style={{ margin: 0 }}>
          Uses the saved settings, so save first if you have just changed them. Any error from the
          relay is shown here in full.
        </p>

        {testResult && (
          <div className={testResult.ok ? "form-success" : "form-error"}>{testResult.text}</div>
        )}

        <div className="row">
          <input
            className="input"
            type="email"
            value={testTo}
            onChange={(event) => setTestTo(event.target.value)}
            placeholder="you@example.com"
            aria-label="Address for the test message"
            style={{ flex: "1 1 16rem" }}
          />
          <button
            type="button"
            className="btn"
            onClick={sendTest}
            disabled={testBusy || !testTo}
          >
            {testBusy ? "Sending…" : "Send test"}
          </button>
        </div>
      </section>

      <section className="card card-pad stack-sm">
        <div className="row-between">
          <h2 style={{ fontSize: "1.05rem" }}>Outbox</h2>
          <span className="tiny faint">
            {mail.pending} waiting · {mail.failed} given up
          </span>
        </div>

        {mail.items.length === 0 ? (
          <p className="small muted" style={{ margin: 0 }}>
            Nothing waiting. Delivered messages are kept for a week.
          </p>
        ) : (
          <div className="list" style={{ margin: "0 -1.25rem -1.25rem" }}>
            {mail.items.map((item) => {
              const givenUp = item.attempts >= (mail.max_attempts || 8);
              const state = givenUp
                ? "given up"
                : item.attempts === 0
                  ? "waiting"
                  : `attempt ${item.attempts} failed`;

              return (
                <div className="list-row" key={item.id}>
                  <div className="list-row-body">
                    <div className="row" style={{ gap: "0.5rem" }}>
                      <span className={`badge ${item.attempts > 0 ? "badge-warn" : ""}`}>{state}</span>
                      <span className="small" style={{ fontWeight: 600 }}>
                        {item.subject}
                      </span>
                    </div>
                  <div className="list-row-meta">
                    <span>to {item.to}</span>
                    <span>{formatDateTime(item.created_at)}</span>
                  </div>
                  {item.last_error && (
                    <p className="form-error" style={{ marginTop: "0.4rem" }}>
                      {item.last_error}
                    </p>
                  )}
                  <div style={{ marginTop: "0.4rem" }}>
                    <button
                      type="button"
                      className="btn btn-ghost btn-sm"
                      onClick={() => void revealBody(item.id)}
                    >
                      {bodies[item.id] === undefined ? "Show contents" : "Hide contents"}
                    </button>
                    <p className="tiny faint" style={{ margin: "0.3rem 0 0" }}>
                      Links are redacted in the list. Showing a message reveals any sign-in link
                      in it, so the action is recorded in the audit log.
                    </p>
                    {bodies[item.id] !== undefined && (
                      <pre
                        className="tiny"
                        style={{
                          whiteSpace: "pre-wrap",
                          background: "var(--bg-inset)",
                          padding: "0.6rem",
                          borderRadius: "6px",
                          marginTop: "0.4rem",
                        }}
                      >
                        {bodies[item.id]}
                      </pre>
                    )}
                  </div>
                  <div className="row" style={{ marginTop: "0.5rem" }}>
                    <button type="button" className="btn btn-sm" onClick={() => act(item.id, "retry")}>
                      Retry now
                    </button>
                    <button
                      type="button"
                      className="btn btn-ghost btn-sm"
                      onClick={() => act(item.id, "delete")}
                    >
                      Discard
                    </button>
                  </div>
                </div>
              </div>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}

function FieldRow({ field, value }: { field: Field; value: string }) {
  if (field.type === "checkbox") {
    return (
      <label className="row" style={{ gap: "0.5rem", alignItems: "flex-start", flexWrap: "nowrap" }}>
        <input
          type="checkbox"
          name={field.key}
          defaultChecked={value === "true"}
          style={{ marginTop: "0.3rem", flex: "none" }}
        />
        {/* flex:1 keeps a long hint from wrapping the label under the box. */}
        <span style={{ flex: 1, minWidth: 0 }}>
          <span style={{ fontWeight: 600 }}>{field.label}</span>
          {field.hint && <span className="hint" style={{ display: "block" }}>{field.hint}</span>}
        </span>
      </label>
    );
  }

  if (field.type === "textarea") {
    return (
      <div className="field">
        <label htmlFor={field.key}>{field.label}</label>
        <textarea
          id={field.key}
          name={field.key}
          className="textarea"
          style={{ minHeight: "3.5rem" }}
          defaultValue={value}
        />
        {field.hint && <span className="hint">{field.hint}</span>}
      </div>
    );
  }

  return (
    <div className="field">
      <label htmlFor={field.key}>{field.label}</label>
      <input
        id={field.key}
        name={field.key}
        className="input"
        type={field.type ?? "text"}
        min={field.type === "number" ? 0 : undefined}
        defaultValue={value}
        placeholder={field.placeholder}
        autoComplete="off"
        style={field.type === "number" ? { maxWidth: "8rem" } : undefined}
      />
      {field.hint && <span className="hint">{field.hint}</span>}
    </div>
  );
}
