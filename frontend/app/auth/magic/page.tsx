import type { Metadata } from "next";

import TokenExchange from "@/components/TokenExchange";

export const metadata: Metadata = { title: "Signing in" };

export default async function MagicLinkPage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  const { token = "" } = await searchParams;
  return <TokenExchange token={token} purpose="magic" />;
}
