import type { Metadata } from "next";

import { getBoards, getMeta } from "@/lib/server";
import NewThreadForm from "@/components/NewThreadForm";

export const metadata: Metadata = { title: "New thread" };

export default async function NewThreadPage({
  searchParams,
}: {
  searchParams: Promise<{ board?: string }>;
}) {
  const { board = "" } = await searchParams;
  const [meta, boards] = await Promise.all([getMeta(), getBoards()]);

  return (
    <div style={{ maxWidth: "46rem", margin: "0 auto" }}>
      <div className="page-head">
        <div>
          <h1>New thread</h1>
          <p>Pick a board, give it a clear title, and write the opening post.</p>
        </div>
      </div>

      <NewThreadForm
        boards={boards}
        initialBoard={board}
        signedIn={!!meta.user}
        maxUploadBytes={meta.max_upload_bytes}
        registrationOpen={meta.allow_registration}
      />
    </div>
  );
}
