// SPDX-License-Identifier: MPL-2.0

import "./app-shell.css";

import type { JSX } from "react";

import { productName } from "./product-name.ts";

/** The empty frame every screen will render inside: one main landmark titled with the product. */
export function AppShell(): JSX.Element {
  return (
    <main className="app-shell">
      <h1 className="app-shell__title">{productName}</h1>
    </main>
  );
}
