// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { readFileSync } from "node:fs";
import path from "node:path";

import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";

import {
  agentHelpFile,
  installGuideOnGitHub,
  llmsTxtFile,
  orpheumInOneSentence,
} from "./agent-help.ts";
import { enDiscovery } from "./locales/en-discovery.ts";

/** The page the core and the loader serve, as the build takes it. */
const page = readFileSync(path.join(import.meta.dirname, "../index.html"), "utf8");

/** Text with each run of spaces and line breaks made one space, as a browser draws it. */
const flowed = (text: string): string => text.replaceAll(/\s+/gu, " ");

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
    ).toEqual([installGuideOnGitHub, `/${agentHelpFile}`, `/${llmsTxtFile}`]);
  });

  // An agent whose fetch ran no script and summarized the page reported it as empty, with no word
  // of what Orpheum is and no way to install it (an agent's install, 2026-11-19).
  it("says what Orpheum is, in the page's description and its root, and links the install guide", () => {
    const served = parsedPage();
    expect(served.head.querySelector(`meta[name="description"]`)?.getAttribute("content")).toBe(
      orpheumInOneSentence,
    );
    const root = served.getElementById("root");
    expect(flowed(root?.textContent ?? "").toLowerCase()).toContain(
      orpheumInOneSentence.toLowerCase(),
    );
    const install = [...(root?.querySelectorAll("a") ?? [])].find(
      (link) => link.getAttribute("href") === installGuideOnGitHub,
    );
    expect(flowed(install?.textContent ?? "").trim()).toBe("Install a server");
  });

  // An agent took the note's instruction for one aimed at it, which it was told to distrust; facts
  // it can check serve it better (an agent's install, 2026-11-20).
  it("says, as facts, that a browser draws more than the text shows, with the install guide's address in its words", () => {
    const text = flowed(parsedPage().getElementById("root")?.textContent ?? "");
    expect(text).toContain("This text is the page as it is served, without its script.");
    expect(text).toContain(`The install guide is at ${installGuideOnGitHub}.`);
    expect(text).toContain(`"${enDiscovery.discovery.realm.install}"`);
  });

  it("shows what Orpheum is and the install guide's link where no script runs", () => {
    const served = parsedPage();
    const shown = served.querySelector("body > noscript");
    expect(shown?.classList.contains("for-agents")).toBe(false);
    // A parser running script holds a noscript's content as text, one running none as elements;
    // its markup parsed again is what a browser without script draws, either way.
    const drawn = new DOMParser().parseFromString(shown?.innerHTML ?? "", "text/html").body;
    expect(flowed(drawn.textContent).toLowerCase()).toContain(orpheumInOneSentence.toLowerCase());
    expect(drawn.querySelector("p")).not.toBeNull();
    expect(drawn.querySelector(".for-agents")).toBeNull();
    const install = drawn.querySelector("a");
    expect(install?.getAttribute("href")).toBe(installGuideOnGitHub);
    expect(flowed(install?.textContent ?? "").trim()).toBe("Install a server");
  });

  it("shows a person none of it while the script starts, as a phone shows each page on the way to its core (the owner, 2026-11-14)", () => {
    const served = parsedPage();
    document.head.innerHTML = [...served.head.querySelectorAll("style")]
      .map((style) => style.outerHTML)
      .join("");
    document.body.innerHTML = served.getElementById("root")?.outerHTML ?? "";
    const note = document.querySelector("#root p");
    if (note === null) {
      throw new Error("the page's root has no note");
    }
    expect(getComputedStyle(note).visibility).toBe("hidden");
    document.head.innerHTML = "";
    document.body.innerHTML = "";
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
