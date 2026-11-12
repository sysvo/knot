import { headers } from "next/headers";

/**
 * SITE_URL is the configured public address of the forum, used for canonical URLs,
 * OpenGraph tags and the sitemap. It must match KNOT_PUBLIC_URL on the API, or
 * shared links point at the wrong host.
 */
export const SITE_URL = (process.env.KNOT_PUBLIC_URL ?? "http://localhost:3000").replace(/\/+$/, "");

/**
 * siteURL returns the address crawlers and shared links should use.
 *
 * The configured value wins. Without it, the request's own host is used, because
 * falling back to localhost publishes a sitemap full of URLs that point at the
 * reader's own machine.
 */
export async function siteURL(): Promise<string> {
  if (process.env.KNOT_PUBLIC_URL) return SITE_URL;
  try {
    const incoming = await headers();
    const host = incoming.get("x-forwarded-host") ?? incoming.get("host");
    if (!host) return SITE_URL;
    const proto =
      incoming.get("x-forwarded-proto") ?? (host.startsWith("localhost") ? "http" : "https");
    return `${proto}://${host}`;
  } catch {
    return SITE_URL;
  }
}
