import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { getMeta } from "@/lib/server";
import SettingsForm from "@/components/SettingsForm";

export const metadata: Metadata = { title: "Settings" };

export default async function SettingsPage() {
  const meta = await getMeta();
  if (!meta.user) redirect("/login?next=/settings");

  return (
    <div className="stack">
      <div className="page-head">
        <div>
          <h1>Settings</h1>
          <p>Your profile, contact details and password.</p>
        </div>
      </div>
      <SettingsForm user={meta.user} maxUploadBytes={meta.max_upload_bytes} />
    </div>
  );
}
