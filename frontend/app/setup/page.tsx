import type { Metadata } from "next";
import { redirect } from "next/navigation";

import SetupForm from "@/components/SetupForm";
import { getMeta } from "@/lib/server";

export const metadata: Metadata = { title: "Set up your forum", robots: { index: false } };

export default async function SetupPage() {
  const meta = await getMeta();

  // Setup is only open while no account exists. Anyone arriving at a running
  // forum belongs on the home page, not on a form that would refuse them.
  if (!meta.needs_setup) redirect(meta.user ? "/" : "/login");

  return (
    <div className="setup-card card card-pad">
      <h1 style={{ fontSize: "1.5rem", marginBottom: "0.25rem" }}>Set up your forum</h1>
      <p className="muted small" style={{ marginBottom: "1.25rem" }}>
        This creates the first account, which is an administrator. Setup closes as soon as it
        exists.
      </p>

      <SetupForm />
    </div>
  );
}
