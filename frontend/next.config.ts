import type { NextConfig } from "next";

// Where the Go API lives. In development the browser only ever talks to the
// Next server, which proxies /api and /uploads through these rewrites, so the
// session cookie stays first-party and no CORS configuration is needed.
//
// In the container image a reverse proxy routes /api and /uploads straight to
// the API, so these rewrites are mostly a development convenience.
const apiUrl = process.env.KNOT_API_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  // A self-contained server bundle, which keeps the container image small.
  output: "standalone",
  poweredByHeader: false,
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${apiUrl}/api/:path*` },
      { source: "/uploads/:path*", destination: `${apiUrl}/uploads/:path*` },
    ];
  },
};

export default nextConfig;
