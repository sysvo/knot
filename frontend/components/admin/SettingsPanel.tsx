"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";

type Field = {
  key: string;
  label: string;
  hint?: string;
  type?: "text" | "textarea" | "checkbox" | "number";
};

const SITE_FIELDS: Field[] = [
  { key: "site_name", label: "Site name", hint: "Shown in the header and page titles." },
  { key: "site_description", label: "Site description", type: "textarea", hint: "Used as the meta description." },
  { key: "allow_registration", label: "Allow registration", type: "checkbox", hint: "Turn off to make the forum invite-only." },
  {
    key: "require_email_verification",
    label: "Require confirmed e-mail",
    type: "checkbox",
    hint: "Needs a mail server; readers must confirm before posting. Has no effect without SMTP.",
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

const ALL_FIELDS = [...SITE_FIELDS, ...SPAM_FIELDS];

/** SettingsPanel edits the site-wide settings. */
export default function SettingsPanel({ settings }: { settings: Record<string, string> }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);

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

  return (
    <form
      className="card card-pad stack"
      style={{ maxWidth: "40rem" }}
      onSubmit={(event) => {
        event.preventDefault();
        void save(event.currentTarget);
      }}
    >
      {message && <div className={message.ok ? "form-success" : "form-error"}>{message.text}</div>}

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

      <div className="row">
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {busy ? "Saving…" : "Save settings"}
        </button>
      </div>
    </form>
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
          style={{ minHeight: "4.5rem" }}
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
        type={field.type === "number" ? "number" : "text"}
        min={field.type === "number" ? 0 : undefined}
        defaultValue={value}
        style={field.type === "number" ? { maxWidth: "8rem" } : undefined}
      />
      {field.hint && <span className="hint">{field.hint}</span>}
    </div>
  );
}
