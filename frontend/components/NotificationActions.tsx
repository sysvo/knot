"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { clientApi } from "@/lib/api";

/** NotificationActions marks notifications read or clears them. */
export default function NotificationActions({ unread }: { unread: number }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);

  async function run(action: "read" | "clear") {
    setBusy(true);
    try {
      if (action === "read") {
        await clientApi("/api/v1/notifications/read", { method: "POST" });
      } else {
        await clientApi("/api/v1/notifications", { method: "DELETE" });
      }
      router.refresh();
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="row">
      <button
        type="button"
        className="btn btn-sm"
        onClick={() => run("read")}
        disabled={busy || unread === 0}
      >
        Mark all read
      </button>
      <button
        type="button"
        className="btn btn-ghost btn-sm"
        onClick={() => run("clear")}
        disabled={busy}
      >
        Clear all
      </button>
    </div>
  );
}
