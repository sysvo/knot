import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { getMeta } from "@/lib/server";
import RegisterForm from "@/components/RegisterForm";

export const metadata: Metadata = { title: "Create an account" };

export default async function RegisterPage() {
  const meta = await getMeta();
  if (meta.user) redirect("/");

  if (!meta.allow_registration) {
    return (
      <div className="auth-card card card-pad center">
        <h1 style={{ fontSize: "1.35rem", marginBottom: "0.5rem" }}>Registration is closed</h1>
        <p className="muted small">
          This forum is not accepting new accounts. If you already have one, you can{" "}
          <Link href="/login">sign in</Link>.
        </p>
      </div>
    );
  }

  return (
    <div className="auth-card card card-pad">
      <h1 style={{ fontSize: "1.5rem", marginBottom: "0.25rem" }}>Create an account</h1>
      <p className="muted small" style={{ marginBottom: "1.25rem" }}>
        Join {meta.site_name}.
      </p>

      <RegisterForm
        providers={meta.providers}
        requireVerification={meta.require_email_verification}
        note={meta.registration_note}
      />
    </div>
  );
}
