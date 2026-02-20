import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: {
          primary: "#0d1117",
          secondary: "#161b22",
          tertiary: "#1c2333",
          elevated: "#21262d",
          hover: "#30363d",
          border: "#30363d",
        },
        text: {
          primary: "#c9d1d9",
          secondary: "#8b949e",
          muted: "#484f58",
        },
        accent: {
          green: "#00ff41",
          amber: "#f0b400",
          blue: "#58a6ff",
          red: "#f85149",
          purple: "#bc8cff",
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
