// Server-side API access, used by Server Components.
//
// Imports "next/headers", so nothing in a client component may import this
// file — use lib/api.ts there instead.

import { cookies } from "next/headers";

import type {
  AdminStats,
  AuditEntry,
  Board,
  Meta,
  Notification,
  PageInfo,
  Post,
  Report,
  SearchHit,
  Self,
  Thread,
  User,
} from "./types";

/** Where the Go API lives. Server-side calls go straight to it. */
export const API_URL = process.env.KNOT_API_URL ?? "http://localhost:8080";

type RequestOptions = {
  method?: string;
  body?: unknown;
};

type ApiErrorShape = { error?: { code?: string; message?: string } };

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const cookieStore = await cookies();
  const headers: Record<string, string> = {
    Accept: "application/json",
    // Forward the visitor's session so the API sees the same user.
    Cookie: cookieStore.toString(),
  };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";

  const response = await fetch(`${API_URL}${path}`, {
    method: options.method ?? "GET",
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    cache: "no-store",
  });

  const text = await response.text();
  const payload = text ? (JSON.parse(text) as unknown) : null;

  if (!response.ok) {
    const detail = (payload as ApiErrorShape | null)?.error;
    throw new Error(detail?.message ?? `request to ${path} failed with ${response.status}`);
  }
  return payload as T;
}

/** serverApi calls the API with the caller's session attached. */
export async function serverApi<T>(path: string, options: RequestOptions = {}): Promise<T> {
  return request<T>(path, options);
}

/**
 * serverApiOr never throws: it returns a fallback instead.
 *
 * A forum's front page should still render when the API hiccups, and a broken
 * widget should not take the whole page down with it.
 */
export async function serverApiOr<T>(path: string, fallback: T): Promise<T> {
  try {
    return await request<T>(path);
  } catch {
    return fallback;
  }
}

const EMPTY_PAGE: PageInfo = { number: 1, per_page: 20, total: 0, has_next: false };

export async function getMeta(): Promise<Meta> {
  return serverApiOr<Meta>("/api/v1/meta", {
    site_name: "knot",
    site_description: "",
    allow_registration: false,
    require_email_verification: false,
    registration_note: "",
    providers: [],
    mail_enabled: false,
    max_upload_bytes: 0,
    version: "dev",
    user: null,
  });
}

export async function getBoards(): Promise<Board[]> {
  const data = await serverApiOr<{ boards: Board[] }>("/api/v1/boards", { boards: [] });
  return data.boards ?? [];
}

export async function getBoard(slug: string): Promise<Board | null> {
  try {
    const data = await request<{ board: Board }>(`/api/v1/boards/${encodeURIComponent(slug)}`);
    return data.board;
  } catch {
    return null;
  }
}

export async function getThreads(
  params: { board?: string; page?: number; sort?: string; perPage?: number } = {},
): Promise<{ threads: Thread[]; page: PageInfo }> {
  const query = new URLSearchParams();
  if (params.page) query.set("page", String(params.page));
  if (params.sort) query.set("sort", params.sort);
  if (params.perPage) query.set("per_page", String(params.perPage));
  const suffix = query.toString() ? `?${query}` : "";

  const path = params.board
    ? `/api/v1/boards/${encodeURIComponent(params.board)}/threads${suffix}`
    : `/api/v1/threads${suffix}`;

  try {
    const data = await request<{ threads: Thread[]; page: PageInfo }>(path);
    return { threads: data.threads ?? [], page: data.page ?? EMPTY_PAGE };
  } catch {
    return { threads: [], page: EMPTY_PAGE };
  }
}

export async function getThread(
  id: number,
  page = 1,
): Promise<{ thread: Thread; posts: Post[]; page: PageInfo } | null> {
  try {
    return await request<{ thread: Thread; posts: Post[]; page: PageInfo }>(
      `/api/v1/threads/${id}?page=${page}`,
    );
  } catch {
    return null;
  }
}

export async function getNotifications(): Promise<{
  notifications: Notification[];
  unread: number;
}> {
  try {
    return await request<{ notifications: Notification[]; unread: number }>(
      "/api/v1/notifications",
    );
  } catch {
    return { notifications: [], unread: 0 };
  }
}

export async function search(q: string, board?: string): Promise<SearchHit[]> {
  const query = new URLSearchParams({ q });
  if (board) query.set("board", board);
  try {
    const data = await request<{ results: SearchHit[] }>(`/api/v1/search?${query}`);
    return data.results ?? [];
  } catch {
    return [];
  }
}

export async function getUser(
  username: string,
): Promise<{ user: User; post_count: number } | null> {
  try {
    return await request<{ user: User; post_count: number }>(
      `/api/v1/users/${encodeURIComponent(username)}`,
    );
  } catch {
    return null;
  }
}

export async function getUserPosts(username: string): Promise<Post[]> {
  try {
    const data = await request<{ posts: Post[] }>(
      `/api/v1/users/${encodeURIComponent(username)}/posts`,
    );
    return data.posts ?? [];
  } catch {
    return [];
  }
}

export async function getAdminStats(): Promise<AdminStats | null> {
  try {
    return await request<AdminStats>("/api/v1/admin/stats");
  } catch {
    return null;
  }
}

export async function getAdminUsers(q = ""): Promise<{ users: User[]; page: PageInfo }> {
  const query = q ? `?q=${encodeURIComponent(q)}` : "";
  try {
    return await request<{ users: User[]; page: PageInfo }>(`/api/v1/admin/users${query}`);
  } catch {
    return { users: [], page: EMPTY_PAGE };
  }
}

export async function getReports(all = false): Promise<Report[]> {
  try {
    const data = await request<{ reports: Report[] }>(
      `/api/v1/admin/reports${all ? "?status=all" : ""}`,
    );
    return data.reports ?? [];
  } catch {
    return [];
  }
}

export async function getSettings(): Promise<Record<string, string>> {
  try {
    const data = await request<{ settings: Record<string, string> }>("/api/v1/admin/settings");
    return data.settings ?? {};
  } catch {
    return {};
  }
}

export async function getAudit(): Promise<AuditEntry[]> {
  try {
    const data = await request<{ entries: AuditEntry[] }>("/api/v1/admin/audit");
    return data.entries ?? [];
  } catch {
    return [];
  }
}

/** getMe is only needed where the page does not already read the site meta. */
export async function getMe(): Promise<Self | null> {
  try {
    return await request<Self>("/api/v1/auth/me");
  } catch {
    return null;
  }
}
