import type { Metadata } from "next";
import "./globals.css";

import { getMeta, getNotifications } from "@/lib/server";
import SiteHeader from "@/components/SiteHeader";
import SiteFooter from "@/components/SiteFooter";

export async function generateMetadata(): Promise<Metadata> {
  const meta = await getMeta();
  return {
    title: {
      default: meta.site_name,
      template: `%s · ${meta.site_name}`,
    },
    description: meta.site_description,
    applicationName: meta.site_name,
    icons: { icon: "/favicon.svg" },
    robots: { index: true, follow: true },
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
        <SiteHeader siteName={meta.site_name} user={meta.user} unread={unread} />
        <main className="site-main">
          <div className="container">{children}</div>
        </main>
        <SiteFooter siteName={meta.site_name} version={meta.version} />
      </body>
    </html>
  );
}
