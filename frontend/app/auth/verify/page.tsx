import type { Metadata } from "next";

import TokenExchange from "@/components/TokenExchange";

export const metadata: Metadata = { title: "Confirming your e-mail" };

export default async function VerifyEmailPage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  const { token = "" } = await searchParams;
  return <TokenExchange token={token} purpose="verify" />;
}
