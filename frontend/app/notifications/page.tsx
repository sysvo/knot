import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { getNotifications, getMeta } from "@/lib/server";
import { displayName } from "@/lib/types";
import Avatar from "@/components/Avatar";
import NotificationActions from "@/components/NotificationActions";
import Timestamp from "@/components/Timestamp";

export const metadata: Metadata = { title: "Notifications" };

export default async function NotificationsPage() {
  const meta = await getMeta();
  if (!meta.user) redirect("/login?next=/notifications");

  const { notifications, unread } = await getNotifications();

  return (
    <div className="stack" style={{ maxWidth: "46rem", margin: "0 auto" }}>
      <div className="page-head">
        <div>
          <h1>Notifications</h1>
          <p>
            {unread > 0
              ? `${unread} unread ${unread === 1 ? "notification" : "notifications"}.`
              : "You are all caught up."}
          </p>
        </div>
        <NotificationActions unread={unread} />
      </div>

      {notifications.length === 0 ? (
        <div className="empty">
          <h2>Nothing yet</h2>
          <p className="small">
            You are notified when someone replies to your thread or mentions you with @username.
          </p>
        </div>
      ) : (
        <div className="list">
          {notifications.map((item) => (
            <div className="notification" key={item.id} data-unread={!item.read}>
              <Avatar user={item.actor} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="row" style={{ gap: "0.4rem" }}>
                  <Link href={`/u/${item.actor?.username ?? ""}`} style={{ fontWeight: 600 }}>
                    {displayName(item.actor)}
                  </Link>
                  <span className="muted small">
                    {item.kind === "mention" ? "mentioned you in" : "replied in"}
                  </span>
                  <Link href={`/t/${item.thread_id}#post-${item.post_id}`} style={{ fontWeight: 600 }}>
                    {item.thread_title || "a thread"}
                  </Link>
                </div>
                {item.excerpt && <p>{item.excerpt.slice(0, 160)}</p>}
                <div className="tiny faint" style={{ marginTop: "0.15rem" }}>
                  <Timestamp at={item.created_at} />
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
