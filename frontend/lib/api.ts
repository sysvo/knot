// Browser-side API access.
//
// This module must stay free of "next/headers" and other server-only imports:
// client components import it, and a stray server import there fails the build
// with a confusing "used in the Pages Router" message.

/** ApiError carries the API's structured error so forms can show it. */
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

type RequestOptions = {
  method?: string;
  body?: unknown;
};

/**
 * clientApi calls the API from the browser.
 *
 * The path is relative, so the request travels through the Next rewrite for
 * /api/* and stays same-origin — which is what lets the session cookie be
 * httpOnly and first-party, with no CORS configuration involved.
 */
export async function clientApi<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";

  const response = await fetch(path, {
    method: options.method ?? "GET",
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    cache: "no-store",
  });

  const text = await response.text();
  const payload = text ? (JSON.parse(text) as unknown) : null;

  if (!response.ok) {
    const detail = (payload as { error?: { code?: string; message?: string } } | null)?.error;
    throw new ApiError(
      response.status,
      detail?.code ?? "error",
      detail?.message ?? `request failed with ${response.status}`,
    );
  }
  return payload as T;
}
