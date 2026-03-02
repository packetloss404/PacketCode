import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";

// Global error handlers — catch unhandled errors and promise rejections
window.addEventListener("unhandledrejection", (event) => {
  console.error("[PacketCode] Unhandled promise rejection:", event.reason);
});

window.addEventListener("error", (event) => {
  console.error(
    "[PacketCode] Unhandled error:",
    event.message,
    "at",
    event.filename,
    ":",
    event.lineno
  );
});

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
