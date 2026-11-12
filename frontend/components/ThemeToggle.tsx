"use client";

import { useEffect, useState } from "react";

type Theme = "light" | "dark";

/** ThemeToggle switches between the light and dark palettes and remembers it. */
export default function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>("light");

  // Read the theme the inline script already applied, so the button starts in
  // the right state without a second source of truth.
  useEffect(() => {
    const current = document.documentElement.getAttribute("data-theme");
    setTheme(current === "dark" ? "dark" : "light");
  }, []);

  function toggle() {
    const next: Theme = theme === "dark" ? "light" : "dark";
    setTheme(next);
    document.documentElement.setAttribute("data-theme", next);
    try {
      localStorage.setItem("knot-theme", next);
    } catch {
      // Private browsing can refuse storage; the theme still applies for the
      // rest of the session.
    }
  }

  return (
    <button
      type="button"
      className="icon-button"
      onClick={toggle}
      aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
      title={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
    >
      {theme === "dark" ? "☀" : "☾"}
    </button>
  );
}
