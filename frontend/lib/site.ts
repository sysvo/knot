/**
 * SITE_URL is the configured public address of the forum, used for canonical URLs,
 * OpenGraph tags and the sitemap. It must match KNOT_PUBLIC_URL on the API, or
 * shared links point at the wrong host.
 *
 * It is deliberately not derived from the request's Host or X-Forwarded-Host: those
 * are attacker-controlled on a direct deployment, and a canonical link or sitemap
 * entry pointing at somebody else's host is a phishing aid. One configured value,
 * one source of truth.
 */
export const SITE_URL = (process.env.KNOT_PUBLIC_URL ?? "http://localhost:3000").replace(/\/+$/, "");
