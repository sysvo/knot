import Link from "next/link";

/** NotFound is shown for unknown threads, boards and profiles. */
export default function NotFound() {
  return (
    <div className="empty">
      <h2>This page does not exist</h2>
      <p className="small">The thread may have been deleted, or the link is wrong.</p>
      <Link href="/" className="btn" style={{ marginTop: "1rem" }}>
        Back to the forum
      </Link>
    </div>
  );
}
