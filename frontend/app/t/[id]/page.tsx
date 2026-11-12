import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { getMeta, getThread } from "@/lib/server";
import { canModerate, displayName } from "@/lib/types";
import { plural } from "@/lib/format";
import Pagination from "@/components/Pagination";
import PostCard from "@/components/PostCard";
import ReplyForm from "@/components/ReplyForm";
import ThreadModBar from "@/components/ThreadModBar";
import Timestamp from "@/components/Timestamp";

type Props = {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ page?: string }>;
};

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params;
  const data = await getThread(Number.parseInt(id, 10));
  return data ? { title: data.thread.title } : { title: "Thread" };
}

export default async function ThreadPage({ params, searchParams }: Props) {
  const { id } = await params;
  const { page: pageParam } = await searchParams;

  const threadId = Number.parseInt(id, 10);
  if (!Number.isFinite(threadId)) notFound();

  const page = Number.parseInt(pageParam ?? "1", 10) || 1;
  const [meta, data] = await Promise.all([getMeta(), getThread(threadId, page)]);
  if (!data) notFound();

  const { thread, posts, page: info } = data;
  const viewer = meta.user;
  const moderator = canModerate(viewer);
  const isAuthor = !!viewer && !!thread.author && viewer.id === thread.author.id;

  return (
    <div className="stack">
      <div>
        <nav className="breadcrumb" aria-label="Breadcrumb">
          <Link href="/">Threads</Link>
          <span aria-hidden="true">/</span>
          <Link href={`/b/${thread.board_slug}`}>{thread.board_name}</Link>
        </nav>

        <h1 style={{ marginBottom: "0.5rem" }}>{thread.title}</h1>

        <div className="row small muted" style={{ gap: "1rem" }}>
          <span>
            started by{" "}
            <Link href={`/u/${thread.author?.username ?? ""}`}>{displayName(thread.author)}</Link>{" "}
            <Timestamp at={thread.created_at} />
          </span>
          <span>{thread.post_count} {plural(thread.post_count, "post")}</span>
          <span>{thread.view_count} {plural(thread.view_count, "view")}</span>
          {thread.pinned && <span className="badge badge-accent">Pinned</span>}
          {thread.locked && <span className="badge badge-warn">Locked</span>}
          {thread.deleted && <span className="badge badge-danger">Deleted</span>}
        </div>
      </div>

      {thread.deleted && (
        <div className="panel small">
          This thread is deleted. Only moderators can see it.
        </div>
      )}

      {(moderator || isAuthor) && !thread.deleted && (
        <ThreadModBar thread={thread} viewerCanModerate={moderator} viewerIsAuthor={isAuthor} />
      )}

      <div className="list">
        {posts.map((post) => (
          <PostCard
            key={post.id}
            post={post}
            viewer={viewer}
            canModerate={moderator}
          />
        ))}
      </div>

      <Pagination page={info} basePath={`/t/${thread.id}`} />

      {!thread.deleted && (
        <section style={{ marginTop: "1rem" }}>
          <ReplyForm
            threadId={thread.id}
            signedIn={!!viewer}
            locked={thread.locked}
            canPostInLocked={moderator}
            maxUploadBytes={meta.max_upload_bytes}
          />
        </section>
      )}
    </div>
  );
}
