import type { MetadataRoute } from "next";

import { siteURL } from "@/lib/site";

// robots.txt names the sitemap URL, so it has to be rendered per request: the
// public host is a runtime fact, and baking it at build time ships an image whose
// robots.txt points every crawler at localhost.
export const dynamic = "force-dynamic";

/**
 * Everything a reader can see is crawlable. Everything that is a page of settings,
 * a moderator tool or a raw JSON endpoint is not.
 */
export default async function robots(): Promise<MetadataRoute.Robots> {
  const base = await siteURL();
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: ["/admin", "/settings", "/notifications", "/api/", "/auth/"],
      },
    ],
    sitemap: `${base}/sitemap.xml`,
    host: base,
  };
}
