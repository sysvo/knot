"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import { formatBytes, formatDateTime, plural } from "@/lib/format";
import type { BackupArchive, Backups } from "@/lib/types";

/**
 * BackupPanel shows the archives on disk and can take one on demand.
 *
 * It says out loud that the files sit beside the database: the most common way to
 * lose a forum is to keep the only copy on the machine that dies.
 */
export default function BackupPanel({ backups }: { backups: Backups }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const latest: BackupArchive | undefined = backups.archives[0];

  async function create() {
    setBusy(true);
    setError(null);
    try {
      await clientApi("/api/v1/admin/backups", { method: "POST" });
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "could not create the backup");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="card card-pad stack-sm">
      <div className="row-between">
        <div>
          <h2 style={{ fontSize: "1.05rem" }}>Backups</h2>
          <p className="muted small">
            {latest
              ? `Last backup ${formatDateTime(latest.created_at)} · ${formatBytes(latest.size)}`
              : "No backup has been taken yet."}
          </p>
        </div>
        <button className="btn btn-sm" onClick={() => void create()} disabled={busy}>
          {busy ? "Backing up…" : "Back up now"}
        </button>
      </div>

      {error && <div className="form-error">{error}</div>}

      <p className="muted small">
        {backups.keep > 0
          ? `A backup is taken automatically once a day, and the newest ${backups.keep} ${
              backups.keep === 1 ? "is" : "are"
            } kept.`
          : "Automatic backups are off (KNOT_BACKUP_KEEP is 0). Back up by hand from here."}{" "}
        Archives are written to <code>{backups.dir || "the data directory"}</code>, next to the
        database — copy them somewhere else too.
      </p>

      {backups.archives.length > 0 && (
        <div className="list">
          {backups.archives.map((archive) => (
            <div className="list-row" key={archive.name}>
              <div className="list-row-body">
                <div className="small">{archive.name}</div>
                <div className="faint small">
                  {formatBytes(archive.size)} · {formatDateTime(archive.created_at)}
                </div>
              </div>
              <a
                className="btn btn-sm"
                href={`/api/v1/admin/backups/${encodeURIComponent(archive.name)}`}
                download
              >
                Download
              </a>
            </div>
          ))}
        </div>
      )}

      <p className="faint small">
        {backups.archives.length} {plural(backups.archives.length, "archive")} on disk. A backup
        contains every account, post and upload, so treat it like the database itself.
      </p>
    </div>
  );
}
