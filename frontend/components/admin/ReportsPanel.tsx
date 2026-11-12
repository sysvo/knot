"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError, clientApi } from "@/lib/api";
import type { Report } from "@/lib/types";
import { displayName } from "@/lib/types";
import Timestamp from "../Timestamp";

/** ReportsPanel is the moderation queue. */
export default function ReportsPanel({ reports, showAll }: { reports: Report[]; showAll: boolean }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function resolve(id: number, status: "resolved" | "dismissed") {
    setBusy(true);
    setError(null);
    try {
      await clientApi(`/api/v1/admin/reports/${id}/resolve`, { method: "POST", body: { status } });
      router.refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "the report was not updated");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="stack">
      {error && <div className="form-error">{error}</div>}

      <div className="row">
        <Link href="/admin?tab=reports" className="btn btn-sm" data-active={!showAll}>
          Open
        </Link>
        <Link href="/admin?tab=reports&amp;status=all" className="btn btn-sm">
          All
        </Link>
      </div>

      {reports.length === 0 ? (
        <div className="empty">
          <h2>Nothing to review</h2>
          <p className="small">Reports filed by readers will appear here.</p>
        </div>
      ) : (
        <div className="list">
          {reports.map((report) => (
            <div className="list-row" key={report.id}>
              <div className="list-row-body">
                <div className="row" style={{ gap: "0.5rem" }}>
                  <span className="badge badge-warn">{report.reason}</span>
                  <span className={`badge ${report.status === "open" ? "" : "badge-ok"}`}>
                    {report.status}
                  </span>
                  <span className="small faint">
                    by {displayName(report.reporter)} <Timestamp at={report.created_at} />
                  </span>
                </div>

                <p className="list-row-title" style={{ marginTop: "0.35rem" }}>
                  {report.post_id ? (
                    <Link href={`/t/${report.thread_id ?? ""}#post-${report.post_id}`}>
                      {report.thread_title || "a thread"}
                    </Link>
                  ) : (
                    <span>{report.thread_title || "a thread"}</span>
                  )}
                </p>

                {report.excerpt && <p className="list-row-excerpt">{report.excerpt}</p>}
                {report.details && <p className="small muted">“{report.details}”</p>}

                {report.status === "open" && (
                  <div className="row" style={{ marginTop: "0.5rem" }}>
                    <button
                      type="button"
                      className="btn btn-sm"
                      disabled={busy}
                      onClick={() => resolve(report.id, "resolved")}
                    >
                      Mark resolved
                    </button>
                    <button
                      type="button"
                      className="btn btn-ghost btn-sm"
                      disabled={busy}
                      onClick={() => resolve(report.id, "dismissed")}
                    >
                      Dismiss
                    </button>
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
