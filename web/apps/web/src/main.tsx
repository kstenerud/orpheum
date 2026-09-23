// SPDX-License-Identifier: MPL-2.0

import "./tokens.css";

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { AppShell } from "./app-shell.tsx";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html has no #root element; the app cannot mount");
}

createRoot(rootElement).render(
  <StrictMode>
    <AppShell />
  </StrictMode>,
);
