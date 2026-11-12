"use client";

import { useRef, useState } from "react";

import type { Upload } from "@/lib/types";

/**
 * MarkdownEditor is a textarea with image attachment.
 *
 * Uploading through the API and inserting a Markdown image link keeps the post
 * itself plain text, so the database stays readable and a broken upload never
 * blocks someone from posting what they wrote.
 */
export default function MarkdownEditor({
  value,
  onChange,
  placeholder,
  rows = 8,
  maxUploadBytes = 0,
  id = "post-body",
  onSubmit,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  rows?: number;
  maxUploadBytes?: number;
  id?: string;
  /** Called on Ctrl+Enter (or Cmd+Enter). */
  onSubmit?: () => void;
}) {
  const ref = useRef<HTMLTextAreaElement>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function insert(text: string) {
    const area = ref.current;
    if (!area) {
      onChange(value + text);
      return;
    }
    const start = area.selectionStart ?? value.length;
    const end = area.selectionEnd ?? value.length;
    const next = value.slice(0, start) + text + value.slice(end);
    onChange(next);

    // Put the caret after the inserted text once React has re-rendered.
    requestAnimationFrame(() => {
      area.focus();
      const caret = start + text.length;
      area.setSelectionRange(caret, caret);
    });
  }

  async function uploadFile(file: File) {
    setError(null);

    if (maxUploadBytes > 0 && file.size > maxUploadBytes) {
      setError(`That image is larger than ${Math.round(maxUploadBytes / (1024 * 1024))} MB.`);
      return;
    }

    setUploading(true);
    try {
      const body = new FormData();
      body.append("file", file);
      // A raw fetch is required here: clientApi sends JSON.
      const response = await fetch("/api/v1/uploads", { method: "POST", body });
      const payload = (await response.json()) as
        | { upload: Upload }
        | { error?: { message?: string } };

      if (!response.ok || !("upload" in payload)) {
        const message =
          "error" in payload && payload.error?.message
            ? payload.error.message
            : "could not upload that image";
        throw new Error(message);
      }

      const name = payload.upload.filename || "image";
      insert(`\n![${name}](${payload.upload.url})\n`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not upload that image");
    } finally {
      setUploading(false);
    }
  }

  return (
    <div className="stack-sm">
      <textarea
        id={id}
        ref={ref}
        className="textarea"
        value={value}
        rows={rows}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={(event) => {
          if (onSubmit && (event.ctrlKey || event.metaKey) && event.key === "Enter") {
            event.preventDefault();
            onSubmit();
          }
        }}
        onPaste={(event) => {
          // Pasting a screenshot straight into the box is the natural way to
          // attach an image, so make it work.
          const file = Array.from(event.clipboardData.files)[0];
          if (file && file.type.startsWith("image/")) {
            event.preventDefault();
            void uploadFile(file);
          }
        }}
      />

      <div className="row-between">
        <span className="tiny faint">Markdown supported. Drag in or paste an image to attach it.</span>
        <label className="btn btn-sm" style={{ cursor: "pointer" }}>
          {uploading ? "Uploading…" : "Attach image"}
          <input
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            hidden
            disabled={uploading}
            onChange={(event) => {
              const file = event.target.files?.[0];
              if (file) void uploadFile(file);
              event.target.value = "";
            }}
          />
        </label>
      </div>

      {error && <div className="form-error">{error}</div>}
    </div>
  );
}
