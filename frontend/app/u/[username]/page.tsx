import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { getMeta, getUser, getUserPosts } from "@/lib/server";
import { displayName } from "@/lib/types";
import { formatDate } from "@/lib/format";
import Avatar from "@/components/Avatar";
import Timestamp from "@/components/Timestamp";

type Props = { params: Promise<{ username: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { username } = await params;
  const data = await getUser(username);
  return data ? { title: `${displayName(data.user)}` } : { title: "Profile" };
}

export default async function ProfilePage({ params }: Props) {
  const { username } = await params;
  const data = await getUser(username);
  if (!data) notFound();

  const [meta, posts] = await Promise.all([getMeta(), getUserPosts(username)]);
  const { user, post_count } = data;
  const isSelf = meta.user?.id === user.id;

  return (
    <div className="stack" style={{ maxWidth: "46rem", margin: "0 auto" }}>
      <div className="card card-pad">
        <div className="row" style={{ gap: "1rem", alignItems: "flex-start" }}>
          <Avatar user={user} size="xl" />
          <div style={{ flex: 1, minWidth: 0 }}>
            <h1 style={{ fontSize: "1.5rem" }}>{displayName(user)}</h1>
            <div className="row small muted" style={{ gap: "0.75rem", marginTop: "0.2rem" }}>
              <span>@{user.username}</span>
              <span>joined {formatDate(user.created_at)}</span>
              <span>
                {post_count} {post_count === 1 ? "post" : "posts"}
              </span>
            </div>
            <div className="row" style={{ gap: "0.35rem", marginTop: "0.5rem" }}>
              {user.role === "admin" && <span className="badge badge-accent">Administrator</span>}
              {user.role === "moderator" && <span className="badge">Moderator</span>}
              {user.status === "banned" && <span className="badge badge-danger">Suspended</span>}
            </div>
            {user.bio && <p style={{ marginTop: "0.75rem", marginBottom: 0 }}>{user.bio}</p>}
          </div>
          {isSelf && (
            <Link href="/settings" className="btn btn-sm">
              Edit profile
            </Link>
          )}
        </div>
      </div>

      <section>
        <h2
          style={{
            fontSize: "0.78rem",
            textTransform: "uppercase",
            letterSpacing: "0.07em",
            color: "var(--text-faint)",
            marginBottom: "0.7rem",
          }}
        >
          Recent posts
        </h2>

        {posts.length === 0 ? (
          <div className="empty">
            <h2>No posts yet</h2>
          </div>
        ) : (
          <div className="list">
            {posts.map((post) => (
              <div className="list-row" key={post.id}>
                <div className="list-row-body">
                  <Link
                    href={`/t/${post.thread_id}#post-${post.id}`}
                    className="list-row-title"
                  >
                    {firstLine(post.body_md)}
                  </Link>
                  <div className="list-row-meta">
                    <Timestamp at={post.created_at} />
                    {post.score !== 0 && <span>{post.score} points</span>}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

/** firstLine is a compact preview of a post for the profile listing. */
function firstLine(markdown: string): string {
  const line = markdown
    .split("\n")
    .map((l) => l.trim())
    .find((l) => l.length > 0 && !l.startsWith("```"));
  const text = (line ?? "(empty)").replace(/^#+\s*/, "").replace(/[*_`>]/g, "");
  return text.length > 90 ? `${text.slice(0, 90)}…` : text;
}
