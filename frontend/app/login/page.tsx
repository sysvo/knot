import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { getMeta } from "@/lib/server";
import LoginForm from "@/components/LoginForm";

export const metadata: Metadata = { title: "Sign in" };

const MESSAGES: Record<string, string> = {
  oauth_denied: "Sign-in was cancelled at the provider.",
  oauth_invalid: "The provider sent back an incomplete response.",
  oauth_state: "That sign-in attempt expired. Please try again.",
  oauth_exchange: "The provider rejected the sign-in. Please try again.",
  oauth_account: "Could not create a local account from that profile.",
  oauth_conflict: "That identity is already linked to another account.",
  oauth_session: "Signed in, but the session could not be created.",
  banned: "This account has been suspended.",
};

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string; next?: string }>;
}) {
  const { error, next } = await searchParams;
  const meta = await getMeta();
  if (meta.user) redirect("/");

  return (
    <div className="auth-card card card-pad">
      <h1 style={{ fontSize: "1.5rem", marginBottom: "0.25rem" }}>Sign in</h1>
      <p className="muted small" style={{ marginBottom: "1.25rem" }}>
        Welcome back to {meta.site_name}.
      </p>

      <LoginForm
        providers={meta.providers}
        mailEnabled={meta.mail_enabled}
        allowRegistration={meta.allow_registration}
        initialError={error ? (MESSAGES[error] ?? "Sign-in failed.") : undefined}
        next={next}
      />

      <hr />
      <p className="tiny faint" style={{ margin: 0 }}>
        <Link href="/">Back to the forum</Link>
      </p>
    </div>
  );
}
