import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { getBoard, getMeta, getThreads } from "@/lib/server";
import Pagination from "@/components/Pagination";
import ThreadList from "@/components/ThreadList";

type Props = {
  params: Promise<{ slug: string }>;
  searchParams: Promise<{ page?: string; sort?: string }>;
};

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params;
  const board = await getBoard(slug);
  return board ? { title: board.name, description: board.description } : { title: "Board" };
}

export default async function BoardPage({ params, searchParams }: Props) {
  const { slug } = await params;
  const { page: pageParam, sort = "recent" } = await searchParams;

  const board = await getBoard(slug);
  if (!board) notFound();

  const page = Number.parseInt(pageParam ?? "1", 10) || 1;
  const [meta, { threads, page: info }] = await Promise.all([
    getMeta(),
    getThreads({ board: slug, page, sort }),
  ]);

  return (
    <div className="stack">
      <div className="page-head">
        <div>
          <nav className="breadcrumb" aria-label="Breadcrumb">
            <Link href="/">Threads</Link>
            <span aria-hidden="true">/</span>
            <span>{board.name}</span>
          </nav>
          <h1>{board.name}</h1>
          {board.description && <p>{board.description}</p>}
        </div>
        {meta.user && (
          <Link href={`/new?board=${board.slug}`} className="btn btn-primary">
            New thread
          </Link>
        )}
      </div>

      <div className="tabs">
        {[
          { key: "recent", label: "Recent activity" },
          { key: "newest", label: "Newest" },
          { key: "popular", label: "Most replies" },
        ].map((tab) => (
          <Link
            key={tab.key}
            href={`/b/${board.slug}?sort=${tab.key}`}
            data-active={sort === tab.key}
          >
            {tab.label}
          </Link>
        ))}
      </div>

      <ThreadList threads={threads} emptyMessage="No threads in this board yet" />
      <Pagination page={info} basePath={`/b/${board.slug}`} params={{ sort }} />
    </div>
  );
}
