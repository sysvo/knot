import type { MetadataRoute } from "next";

import { publicApi } from "@/lib/server";
import { SITE_URL } from "@/lib/site";

type SitemapData = {
  threads: { id: number; last_post_at: number }[];
  boards: { slug: string; last_post_at: number }[];
};

/** stamp turns a Unix timestamp into a date, or nothing if it is not one. */
function stamp(seconds: number): Date | undefined {
  return Number.isFinite(seconds) && seconds > 0 ? new Date(seconds * 1000) : undefined;
}

/**
 * The sitemap is generated from the API so it stays correct on a busy forum.
 * Static routes are listed here because Next has no other way to know about them.
 */
export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const data = await publicApi<SitemapData>("/api/v1/sitemap", { threads: [], boards: [] });
  const base = SITE_URL;

  return [
    { url: `${base}/`, changeFrequency: "hourly", priority: 1 },
    { url: `${base}/search`, changeFrequency: "weekly", priority: 0.3 },
    ...data.boards.map((board) => ({
      url: `${base}/b/${board.slug}`,
      lastModified: stamp(board.last_post_at),
      changeFrequency: "daily" as const,
      priority: 0.8,
    })),
    ...data.threads.map((thread) => ({
      url: `${base}/t/${thread.id}`,
      lastModified: stamp(thread.last_post_at),
      changeFrequency: "daily" as const,
      priority: 0.6,
    })),
  ];
}
