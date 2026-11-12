import type { Metadata } from "next";

import { getMeta } from "@/lib/server";
import ForgotForm from "@/components/ForgotForm";

export const metadata: Metadata = { title: "Reset your password" };

export default async function ForgotPage() {
  const meta = await getMeta();

  return (
    <div className="auth-card card card-pad">
      <h1 style={{ fontSize: "1.5rem", marginBottom: "0.25rem" }}>Reset your password</h1>
      <p className="muted small" style={{ marginBottom: "1.25rem" }}>
        {meta.mail_enabled
          ? "Enter the address on your account and we will send a link."
          : "This instance has no mail server configured, so passwords cannot be reset by e-mail. Ask an administrator to run `knot passwd` for you."}
      </p>

      {meta.mail_enabled ? (
        <ForgotForm />
      ) : (
        <p className="small" style={{ margin: 0 }}>
          <a href="/login">Back to sign in</a>
        </p>
      )}
    </div>
  );
}
