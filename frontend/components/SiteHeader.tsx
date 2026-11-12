"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { clientApi } from "@/lib/api";
import type { Self } from "@/lib/types";

import Avatar from "./Avatar";
import ThemeToggle from "./ThemeToggle";

/**
 * SiteHeader renders the top bar. It is a client component because the menu and
 * sign-out need interactivity, but all of its data is rendered on the server
 * and passed in, so there is no client-side fetch on load.
 */
export default function SiteHeader({
  siteName,
  user,
  unread,
}: {
  siteName: string;
  user: Self | null;
  unread: number;
}) {
  const router = useRouter();
  const [signingOut, setSigningOut] = useState(false);

  async function signOut() {
    setSigningOut(true);
    try {
      await clientApi("/api/v1/auth/logout", { method: "POST" });
      router.push("/");
      router.refresh();
    } finally {
      setSigningOut(false);
    }
  }

  return (
    <header className="site-header">
      <div className="container site-header-inner">
        <Link href="/" className="brand">
          <span className="brand-mark" aria-hidden="true">
            {siteName.slice(0, 1).toUpperCase()}
          </span>
          <span>{siteName}</span>
        </Link>

        <nav className="site-nav" aria-label="Main">
          <Link href="/">Threads</Link>
          {user && <Link href="/new">New thread</Link>}
        </nav>

        <form className="search-form" action="/search" method="get" role="search">
          <span className="search-icon" aria-hidden="true">
            ⌕
          </span>
          <label htmlFor="header-search" className="sr-only">
            Search posts
          </label>
          <input
            id="header-search"
            type="search"
            name="q"
            placeholder="Search…"
            autoComplete="off"
            maxLength={120}
          />
        </form>

        <div className="row" style={{ gap: "0.35rem" }}>
          <ThemeToggle />

          {user ? (
            <>
              <Link
                href="/notifications"
                className="icon-button"
                aria-label={unread > 0 ? `Notifications, ${unread} unread` : "Notifications"}
                title="Notifications"
              >
                <span aria-hidden="true">◔</span>
                {unread > 0 && (
                  <span className="badge-dot">{unread > 99 ? "99+" : unread}</span>
                )}
              </Link>

              <details className="menu">
                <summary>
                  <Avatar user={user} />
                  <span className="small muted" style={{ paddingRight: "0.35rem" }}>
                    {user.username}
                  </span>
                </summary>
                <div className="menu-panel">
                  <Link href={`/u/${user.username}`}>Your profile</Link>
                  <Link href="/settings">Settings</Link>
                  {user.role === "admin" && <Link href="/admin">Administration</Link>}
                  <hr />
                  <button type="button" onClick={signOut} disabled={signingOut}>
                    {signingOut ? "Signing out…" : "Sign out"}
                  </button>
                </div>
              </details>
            </>
          ) : (
            <>
              <Link href="/login" className="btn btn-ghost btn-sm">
                Sign in
              </Link>
              <Link href="/register" className="btn btn-primary btn-sm">
                Join
              </Link>
            </>
          )}
        </div>
      </div>
    </header>
  );
}
