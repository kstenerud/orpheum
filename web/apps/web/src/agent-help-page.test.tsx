// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { readFileSync } from "node:fs";
import path from "node:path";

import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";

import { agentHelpFile, llmsTxtFile } from "./agent-help.ts";

/** The page the core and the loader serve, as the build takes it. */
const page = readFileSync(path.join(import.meta.dirname, "../index.html"), "utf8");

/** The page as a browser parses it, before any script runs. */
function parsedPage(): Document {
  return new DOMParser().parseFromString(page, "text/html");
}

describe("the page's naming of the setup help for a person's AI agent", () => {
  it("is in the page's head, as help and as its Markdown alternate", () => {
    const head = parsedPage().head;
    expect(head.querySelector(`link[rel="help"]`)?.getAttribute("href")).toBe(`/${agentHelpFile}`);
    const alternate = head.querySelector(`link[rel="alternate"][type="text/markdown"]`);
    expect(alternate?.getAttribute("href")).toBe(`/${agentHelpFile}`);
  });

  it("is in plain text in the page's root, for an agent that reads only the text", () => {
    const root = parsedPage().getElementById("root");
    expect(root?.textContent).toContain(`/${agentHelpFile}`);
    expect(root?.textContent).toContain(`/${llmsTxtFile}`);
    expect(
      [...(root?.querySelectorAll("a") ?? [])].map((link) => link.getAttribute("href")),
    ).toEqual([`/${agentHelpFile}`, `/${llmsTxtFile}`]);
  });

  it("is gone once the client draws, for a person and a screen reader alike", async () => {
    Reflect.set(globalThis, "IS_REACT_ACT_ENVIRONMENT", true);
    // The page's root as it is served, without the script, which happy-dom would try to load.
    document.body.innerHTML = parsedPage().getElementById("root")?.outerHTML ?? "";
    const rootElement = document.getElementById("root");
    if (rootElement === null) {
      throw new Error("the page has no #root");
    }
    expect(document.body.textContent).toContain(agentHelpFile);
    const root = createRoot(rootElement);
    // eslint-disable-next-line @typescript-eslint/require-await -- act's async form flushes the commit
    await act(async () => {
      root.render(<main />);
    });
    expect(document.body.textContent).not.toContain(agentHelpFile);
    expect(rootElement.innerHTML).toBe("<main></main>");
    act(() => {
      root.unmount();
    });
  });
});
