"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";

const FIELDS: { key: string; label: string; hint?: string; type?: "text" | "textarea" | "checkbox" }[] = [
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
    for (const field of FIELDS) {
      payload[field.key] =
        field.type === "checkbox" ? (data.get(field.key) ? "true" : "false") : String(data.get(field.key) ?? "");
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
      className="card card-pad stack-sm"
      style={{ maxWidth: "36rem" }}
      onSubmit={(event) => {
        event.preventDefault();
        void save(event.currentTarget);
      }}
    >
      {message && <div className={message.ok ? "form-success" : "form-error"}>{message.text}</div>}

      {FIELDS.map((field) =>
        field.type === "checkbox" ? (
          <label key={field.key} className="row" style={{ gap: "0.5rem", alignItems: "flex-start" }}>
            <input
              type="checkbox"
              name={field.key}
              defaultChecked={settings[field.key] === "true"}
              style={{ marginTop: "0.3rem" }}
            />
            <span>
              <span style={{ fontWeight: 600 }}>{field.label}</span>
              {field.hint && <span className="hint" style={{ display: "block" }}>{field.hint}</span>}
            </span>
          </label>
        ) : field.type === "textarea" ? (
          <div className="field" key={field.key}>
            <label htmlFor={field.key}>{field.label}</label>
            <textarea
              id={field.key}
              name={field.key}
              className="textarea"
              style={{ minHeight: "4.5rem" }}
              defaultValue={settings[field.key] ?? ""}
            />
            {field.hint && <span className="hint">{field.hint}</span>}
          </div>
        ) : (
          <div className="field" key={field.key}>
            <label htmlFor={field.key}>{field.label}</label>
            <input
              id={field.key}
              name={field.key}
              className="input"
              defaultValue={settings[field.key] ?? ""}
            />
            {field.hint && <span className="hint">{field.hint}</span>}
          </div>
        ),
      )}

      <div className="row">
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {busy ? "Saving…" : "Save settings"}
        </button>
      </div>
    </form>
  );
}
