"use client";

import Link from "next/link";
import { useEffect } from "react";

/** Error boundary for unexpected render failures. */
export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // The API is a separate process, so the useful detail is usually here.
    console.error(error);
  }, [error]);

  return (
    <div className="empty">
      <h2>Something went wrong</h2>
      <p className="small">
        {error.message || "The page could not be rendered."}
        {error.digest ? ` (${error.digest})` : ""}
      </p>
      <div className="row" style={{ justifyContent: "center", marginTop: "1rem" }}>
        <button type="button" className="btn btn-primary" onClick={reset}>
          Try again
        </button>
        <Link href="/" className="btn">
          Back to the forum
        </Link>
      </div>
    </div>
  );
}
