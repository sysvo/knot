import type { Metadata } from "next";

import ResetForm from "@/components/ResetForm";

export const metadata: Metadata = { title: "Choose a new password" };

export default async function ResetPage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  const { token = "" } = await searchParams;

  return (
    <div className="auth-card card card-pad">
      <h1 style={{ fontSize: "1.5rem", marginBottom: "0.25rem" }}>Choose a new password</h1>
      <p className="muted small" style={{ marginBottom: "1.25rem" }}>
        Signing in with the new password will sign out every other device.
      </p>
      <ResetForm token={token} />
    </div>
  );
}
