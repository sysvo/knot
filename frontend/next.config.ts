import type { NextConfig } from "next";

// Where the Go API lives. In development the browser only ever talks to the
// Next server, which proxies /api and /uploads through these rewrites, so the
// session cookie stays first-party and no CORS configuration is needed.
//
// In the container image a reverse proxy routes /api and /uploads straight to
// the API, so these rewrites are mostly a development convenience.
const apiUrl = process.env.KNOT_API_URL ?? "http://localhost:8080";

// Headers for the HTML application. The API sets its own on its own responses, and
// /api and /uploads are excluded here so those are never overwritten — an uploaded
// image is deliberately served with a locked-down CSP that would be lost if this
// policy applied to it.
//
// script-src keeps 'unsafe-inline' because the theme bootstrap and Next's own
// hydration payload are inline; the policy still blocks externally hosted script,
// plugins, <base> hijacking and form exfiltration, and frame-ancestors is what
// stops the admin panel being framed. See docs/ARCHITECTURE.md for what it does
// not cover.
const contentSecurityPolicy = [
  "default-src 'self'",
  "base-uri 'self'",
  "object-src 'none'",
  "frame-ancestors 'none'",
  "form-action 'self'",
  "img-src 'self' data: https:",
  "font-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  // The dev server evaluates modules at run time; production does not need it.
  process.env.NODE_ENV === "production"
    ? "script-src 'self' 'unsafe-inline'"
    : "script-src 'self' 'unsafe-inline' 'unsafe-eval'",
  "connect-src 'self'",
].join("; ");

const appHeaders = [
  { key: "Content-Security-Policy", value: contentSecurityPolicy },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
];

const nextConfig: NextConfig = {
  // A self-contained server bundle, which keeps the container image small.
  output: "standalone",
  poweredByHeader: false,
  async headers() {
    return [
      { source: "/", headers: appHeaders },
      { source: "/:path((?!api/|uploads/).*)", headers: appHeaders },
    ];
  },
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${apiUrl}/api/:path*` },
      { source: "/uploads/:path*", destination: `${apiUrl}/uploads/:path*` },
    ];
  },
};

export default nextConfig;
