import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import {
  getAdminStats,
  getAdminUsers,
  getAudit,
  getBackups,
  getBoards,
  getMailQueue,
  getMeta,
  getPending,
  getReports,
  getSettings,
} from "@/lib/server";
import { canModerate, isAdmin } from "@/lib/types";
import { formatDateTime } from "@/lib/format";
import BackupPanel from "@/components/admin/BackupPanel";
import BoardsPanel from "@/components/admin/BoardsPanel";
import PendingPanel from "@/components/admin/PendingPanel";
import ReportsPanel from "@/components/admin/ReportsPanel";
import SettingsPanel from "@/components/admin/SettingsPanel";
import UsersPanel from "@/components/admin/UsersPanel";

export const metadata: Metadata = { title: "Administration" };

type Tab = "overview" | "boards" | "users" | "review" | "reports" | "settings" | "audit";

const ADMIN_TABS: { key: Tab; label: string }[] = [
  { key: "overview", label: "Overview" },
  { key: "boards", label: "Boards" },
  { key: "users", label: "Users" },
  { key: "review", label: "Review" },
  { key: "reports", label: "Reports" },
  { key: "settings", label: "Settings" },
  { key: "audit", label: "Audit log" },
];

const MODERATOR_TABS: { key: Tab; label: string }[] = [
  { key: "review", label: "Review" },
  { key: "reports", label: "Reports" },
  { key: "audit", label: "Audit log" },
];

export default async function AdminPage({
  searchParams,
}: {
  searchParams: Promise<{ tab?: string; status?: string; q?: string }>;
}) {
  const { tab: rawTab, status, q } = await searchParams;
  const meta = await getMeta();

  if (!meta.user) redirect("/login?next=/admin");
  if (!canModerate(meta.user)) {
    return (
      <div className="empty">
        <h2>Not for you</h2>
        <p className="small">This area is for moderators and administrators.</p>
        <Link href="/" className="btn" style={{ marginTop: "1rem" }}>
          Back to the forum
        </Link>
      </div>
    );
  }

  const admin = isAdmin(meta.user);
  const tabs = admin ? ADMIN_TABS : MODERATOR_TABS;
  const requested = (rawTab ?? tabs[0]?.key ?? "reports") as Tab;
  const tab = tabs.some((t) => t.key === requested) ? requested : (tabs[0]?.key ?? "reports");

  return (
    <div className="stack">
      <div className="page-head">
        <div>
          <h1>Administration</h1>
          <p>{admin ? "Everything about this instance." : "The moderation queue."}</p>
        </div>
      </div>

      <nav className="tabs" aria-label="Administration sections">
        {tabs.map((item) => (
          <Link key={item.key} href={`/admin?tab=${item.key}`} data-active={item.key === tab}>
            {item.label}
          </Link>
        ))}
      </nav>

      {tab === "overview" && admin && <Overview />}
      {tab === "boards" && admin && <BoardsTab />}
      {tab === "users" && admin && <UsersTab query={q ?? ""} />}
      {tab === "review" && <ReviewTab />}
      {tab === "reports" && <ReportsTab showAll={status === "all"} />}
      {tab === "settings" && admin && <SettingsTab />}
      {tab === "audit" && <AuditTab />}
    </div>
  );
}

async function Overview() {
  const [stats, reports, mail, settings, backups] = await Promise.all([
    getAdminStats(),
    getReports(),
    getMailQueue(),
    getSettings(),
    getBackups(),
  ]);

  if (!stats) {
    return <div className="form-error">Could not load the statistics.</div>;
  }

  return (
    <div className="stack">
      <div className="stats-grid">
        <div className="stat">
          <div className="stat-value">{stats.users}</div>
          <div className="stat-label">Accounts</div>
        </div>
        <div className="stat">
          <div className="stat-value">{stats.threads}</div>
          <div className="stat-label">Threads</div>
        </div>
        <div className="stat">
          <div className="stat-value">{stats.posts}</div>
          <div className="stat-label">Posts</div>
        </div>
        <div className="stat">
          <div className="stat-value">{stats.open_reports}</div>
          <div className="stat-label">Open reports</div>
        </div>
        <div className="stat">
          <div className="stat-value">{stats.pending_items}</div>
          <div className="stat-label">Awaiting review</div>
        </div>
        <div className="stat">
          <div className="stat-value">{stats.banned_users}</div>
          <div className="stat-label">Suspended</div>
        </div>
      </div>

      {(reports.length > 0 || stats.pending_items > 0) && (
        <div className="panel stack-sm small">
          {reports.length > 0 && (
            <span>
              <strong>{reports.length}</strong>{" "}
              <Link href="/admin?tab=reports">
                report{reports.length === 1 ? "" : "s"} waiting for review
              </Link>
              .
            </span>
          )}
          {stats.pending_items > 0 && (
            <span>
              <strong>{stats.pending_items}</strong>{" "}
              <Link href="/admin?tab=review">
                item{stats.pending_items === 1 ? "" : "s"} held for review
              </Link>
              .
            </span>
          )}
        </div>
      )}

      {/* A settings combination that silently locks people out belongs on the
          dashboard, not in a documentation page. */}
      {settings.require_email_verification === "true" && !mail.configured && (
        <div className="form-error">
          <strong>New members cannot post.</strong> “Require confirmed e-mail” is on, but no mail
          server is configured, so nobody can confirm their address.{" "}
          <Link href="/admin?tab=settings">Set up e-mail</Link> or turn the requirement off.
        </div>
      )}

      {(!mail.configured || mail.failed > 0) && (
        <div className="panel stack-sm small">
          {!mail.configured && (
            <span>
              <strong>E-mail is not configured.</strong> Magic links and password resets are
              unavailable{mail.pending > 0 ? `, and ${mail.pending} messages are waiting` : ""}.{" "}
              <Link href="/admin?tab=settings">Set up SMTP</Link>.
            </span>
          )}
          {mail.failed > 0 && (
            <span>
              <strong>
                {mail.failed} message{mail.failed === 1 ? "" : "s"} could not be delivered.
              </strong>{" "}
              <Link href="/admin?tab=settings">See the outbox</Link>.
            </span>
          )}
        </div>
      )}

      <BackupPanel backups={backups} />
    </div>
  );
}

async function BoardsTab() {
  const boards = await getBoards();
  return <BoardsPanel boards={boards} />;
}

async function UsersTab({ query }: { query: string }) {
  const { users } = await getAdminUsers(query);
  return <UsersPanel users={users} query={query} />;
}

async function ReportsTab({ showAll }: { showAll: boolean }) {
  const reports = await getReports(showAll);
  return <ReportsPanel reports={reports} showAll={showAll} />;
}

async function ReviewTab() {
  const { threads, posts } = await getPending();
  return <PendingPanel threads={threads} posts={posts} />;
}

async function SettingsTab() {
  const [settings, mail] = await Promise.all([getSettings(), getMailQueue()]);
  return <SettingsPanel settings={settings} mail={mail} />;
}

async function AuditTab() {
  const entries = await getAudit();

  if (entries.length === 0) {
    return (
      <div className="empty">
        <h2>No actions recorded yet</h2>
      </div>
    );
  }

  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            <th>When</th>
            <th>Who</th>
            <th>Action</th>
            <th>Target</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry) => (
            <tr key={entry.id}>
              <td className="small nowrap">{formatDateTime(entry.created_at)}</td>
              <td className="small">{entry.actor?.username ?? "system"}</td>
              <td className="small mono">{entry.action}</td>
              <td className="small faint">
                {entry.target_type}
                {entry.target_id ? ` #${entry.target_id}` : ""}
                {entry.details ? ` — ${entry.details}` : ""}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
