// SPDX-License-Identifier: MPL-2.0

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AppShell } from "./app-shell.tsx";

describe("AppShell", () => {
  it("renders one main landmark headed by the product name", () => {
    const markup = renderToStaticMarkup(<AppShell />);

    expect(markup).toMatch(/^<main[^>]*>/);
    expect(markup).toContain(">Orpheum</h1>");
  });
});
