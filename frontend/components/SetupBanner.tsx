"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

/**
 * SetupBanner points at the setup page from anywhere except the setup page.
 *
 * It is a client component only so it can read the pathname: showing "finish
 * setup" on top of the setup form itself is a button that does nothing.
 */
export default function SetupBanner({ siteName }: { siteName: string }) {
  const pathname = usePathname();
  if (pathname === "/setup") return null;

  return (
    <div className="setup-banner">
      <div className="container setup-banner-inner">
        <span>
          <strong>{siteName}</strong> has no administrator yet.
        </span>
        <Link className="btn btn-sm btn-primary" href="/setup">
          Finish setup
        </Link>
      </div>
    </div>
  );
}
