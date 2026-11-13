import type { MetadataRoute } from "next";

import { SITE_URL } from "@/lib/site";

// Rendered per request rather than at build time: the public host comes from
// KNOT_PUBLIC_URL, which a container image only has at run time. Baking it in
// would ship a robots.txt that points every crawler at localhost.
export const dynamic = "force-dynamic";

/**
 * Everything a reader can see is crawlable. Everything that is a page of settings,
 * a moderator tool or a raw JSON endpoint is not.
 */
export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: ["/admin", "/settings", "/notifications", "/api/", "/auth/"],
      },
    ],
    sitemap: `${SITE_URL}/sitemap.xml`,
    host: SITE_URL,
  };
}
