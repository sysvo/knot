import type { Metadata } from "next";
import "./globals.css";

import { getMeta, getNotifications } from "@/lib/server";
import { siteURL } from "@/lib/site";
import SetupBanner from "@/components/SetupBanner";
import SiteHeader from "@/components/SiteHeader";
import SiteFooter from "@/components/SiteFooter";

export async function generateMetadata(): Promise<Metadata> {
  // Relative URLs in OpenGraph tags and canonicals resolve against this base, so
  // it has to be the same address the sitemap and robots.txt use — otherwise a
  // shared link advertises a host the reader cannot reach.
  const [meta, base] = await Promise.all([getMeta(), siteURL()]);
  return {
    metadataBase: new URL(base),
    title: {
      default: meta.site_name,
      template: `%s · ${meta.site_name}`,
    },
    description: meta.site_description,
    applicationName: meta.site_name,
    icons: { icon: "/favicon.svg" },
    // Shared links get a title, a description and a site name instead of a bare URL.
    openGraph: {
      type: "website",
      siteName: meta.site_name,
      title: meta.site_name,
      description: meta.site_description,
      url: "/",
    },
    twitter: { card: "summary" },
    robots: meta.needs_setup ? { index: false, follow: false } : { index: true, follow: true },
  };
}

/**
 * themeScript applies the stored theme before first paint. Without it, a reader
 * who prefers dark mode sees a white flash on every navigation.
 */
const themeScript = `
(function () {
  try {
    var stored = localStorage.getItem("knot-theme");
    var theme = stored === "light" || stored === "dark"
      ? stored
      : (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
    document.documentElement.setAttribute("data-theme", theme);
  } catch (e) {
    document.documentElement.setAttribute("data-theme", "light");
  }
})();
`;

export default async function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const meta = await getMeta();
  const { unread } = meta.user ? await getNotifications() : { unread: 0 };

  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>
        {meta.needs_setup && <SetupBanner siteName={meta.site_name} />}
        <SiteHeader
          siteName={meta.site_name}
          user={meta.user}
          unread={unread}
          needsSetup={meta.needs_setup}
        />
        <main className="site-main">
          <div className="container">{children}</div>
        </main>
        <SiteFooter siteName={meta.site_name} version={meta.version} />
      </body>
    </html>
  );
}
