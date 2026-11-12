"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { ApiError, clientApi } from "@/lib/api";

/**
 * TokenExchange consumes a single-use token (a magic link or an e-mail
 * verification link) and finishes the sign-in.
 *
 * It runs in the browser rather than during a server render because the API
 * responds with Set-Cookie, and only a browser request can store that. The ref
 * guard keeps React's development double-invocation from burning the token.
 */
export default function TokenExchange({
  token,
  purpose,
}: {
  token: string;
  purpose: "magic" | "verify";
}) {
  const router = useRouter();
  const started = useRef(false);
  const [state, setState] = useState<"working" | "done" | "failed">("working");
  const [message, setMessage] = useState("");

  useEffect(() => {
    if (started.current) return;
    started.current = true;

    if (!token) {
      setState("failed");
      setMessage("This link is missing its token.");
      return;
    }

    const path = purpose === "magic" ? "/api/v1/auth/magic/consume" : "/api/v1/auth/verify";
    void (async () => {
      try {
        await clientApi(path, { method: "POST", body: { token } });
        setState("done");
        setMessage(
          purpose === "magic" ? "You are signed in. Taking you to the forum…" : "Your e-mail is confirmed.",
        );
        router.refresh();
        if (purpose === "magic") {
          window.setTimeout(() => router.push("/"), 700);
        }
      } catch (err) {
        setState("failed");
        setMessage(err instanceof ApiError ? err.message : "that link did not work");
      }
    })();
  }, [token, purpose, router]);

  return (
    <div className="auth-card card card-pad">
      <h1 style={{ fontSize: "1.35rem", marginBottom: "0.6rem" }}>
        {state === "failed" ? "Link did not work" : state === "done" ? "All set" : "One moment…"}
      </h1>
      <p className={state === "failed" ? "form-error" : "muted"}>{message}</p>
      {state !== "working" && (
        <p className="small" style={{ marginTop: "1rem", marginBottom: 0 }}>
          <Link href="/login">Back to sign in</Link>
        </p>
      )}
    </div>
  );
}
