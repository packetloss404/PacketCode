import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: {
          primary: "var(--color-bg-primary, #0d1117)",
          secondary: "var(--color-bg-secondary, #161b22)",
          tertiary: "var(--color-bg-tertiary, #1c2333)",
          elevated: "var(--color-bg-elevated, #21262d)",
          hover: "var(--color-bg-hover, #30363d)",
          border: "var(--color-bg-border, #30363d)",
        },
        text: {
          primary: "var(--color-text-primary, #c9d1d9)",
          secondary: "var(--color-text-secondary, #8b949e)",
          muted: "var(--color-text-muted, #484f58)",
        },
        accent: {
          green: "var(--color-accent-green, #00ff41)",
          amber: "var(--color-accent-amber, #f0b400)",
          blue: "var(--color-accent-blue, #58a6ff)",
          red: "var(--color-accent-red, #f85149)",
          purple: "var(--color-accent-purple, #bc8cff)",
        },
      },
      fontFamily: {
        mono: [
          "JetBrains Mono",
          "Cascadia Code",
          "Fira Code",
          "Consolas",
          "monospace",
        ],
      },
    },
  },
  plugins: [],
} satisfies Config;
