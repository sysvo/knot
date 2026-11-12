// Types mirroring the JSON produced by the Go API. Keeping them in one file
// means a schema change shows up as a compile error rather than a runtime
// surprise.

export type Role = "member" | "moderator" | "admin";
export type AccountStatus = "active" | "banned";

export interface User {
  id: number;
  username: string;
  display_name: string;
  bio: string;
  avatar_url: string;
  role: Role;
  status: AccountStatus;
  created_at: number;
}

export interface Self extends User {
  email: string;
  email_verified: boolean;
  has_password: boolean;
  providers: string[];
}

export interface Board {
  id: number;
  slug: string;
  name: string;
  description: string;
  position: number;
  created_at: number;
  thread_count: number;
  post_count: number;
  last_post_at: number;
}

export interface Thread {
  id: number;
  board_id: number;
  board_slug: string;
  board_name: string;
  title: string;
  pinned: boolean;
  locked: boolean;
  deleted: boolean;
  author: User | null;
  last_author: User | null;
  excerpt: string;
  post_count: number;
  view_count: number;
  created_at: number;
  last_post_at: number;
}

export interface Post {
  id: number;
  thread_id: number;
  author: User | null;
  body_md: string;
  body_html: string;
  score: number;
  my_vote: number;
  deleted: boolean;
  created_at: number;
  edited_at: number | null;
  hidden: boolean;
}

export interface Notification {
  id: number;
  kind: "reply" | "mention" | string;
  actor: User | null;
  thread_id: number;
  thread_title: string;
  post_id: number;
  excerpt: string;
  read: boolean;
  created_at: number;
}

export interface Report {
  id: number;
  reporter: User | null;
  post_id: number | null;
  thread_id: number | null;
  thread_title: string;
  excerpt: string;
  reason: string;
  details: string;
  status: "open" | "resolved" | "dismissed";
  created_at: number;
}

export interface AuditEntry {
  id: number;
  action: string;
  target_type: string;
  target_id: number;
  details: string;
  created_at: number;
  actor: User | null;
}

export interface SearchHit {
  thread_id: number;
  post_id: number;
  title: string;
  board_slug: string;
  board_name: string;
  snippet: string;
  author: string;
  last_post_at: number;
  created_at: number;
}

export interface Upload {
  id: number;
  url: string;
  filename: string;
  content_type: string;
  size: number;
}

export interface PageInfo {
  number: number;
  per_page: number;
  total: number;
  has_next: boolean;
}

export interface Meta {
  site_name: string;
  site_description: string;
  allow_registration: boolean;
  require_email_verification: boolean;
  registration_note: string;
  providers: string[];
  mail_enabled: boolean;
  max_upload_bytes: number;
  version: string;
  user: Self | null;
}

export interface AdminStats {
  users: number;
  threads: number;
  posts: number;
  open_reports: number;
  banned_users: number;
  published_at: number;
}

/** The name to show in the interface. */
export function displayName(user: User | null): string {
  if (!user) return "[deleted]";
  return user.display_name || user.username;
}

/** Whether the user may act on other people's content. */
export function canModerate(user: User | null): boolean {
  return !!user && (user.role === "moderator" || user.role === "admin");
}

export function isAdmin(user: User | null): boolean {
  return !!user && user.role === "admin";
}
