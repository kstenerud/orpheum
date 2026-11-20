// SPDX-License-Identifier: MPL-2.0

import { readFileSync } from "node:fs";
import path from "node:path";

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  agentHelpFile,
  agentHelpSource,
  buildMetaName,
  buildOf,
  installFacts,
  installGuideOnGitHub,
  llmsTxt,
  llmsTxtFile,
  llmsTxtOf,
  servedAgentHelp,
} from "./agent-help.ts";

/** The help's one source, under docs/users/. */
const helpSource = readFileSync(
  path.join(import.meta.dirname, "../../../..", agentHelpSource),
  "utf8",
);

/** The build a test builds as, given the build as orpheum-connect's package gives it. */
const givenBuild = "b8266a2-7975fb51d";

/** The plugin vite.config.ts builds with, the build given as ORPHEUM_BUILD. */
async function helpPlugin(): Promise<Record<string, unknown>> {
  vi.stubEnv("ORPHEUM_BUILD", givenBuild);
  const { default: config } = await import("../vite.config.ts");
  const resolved = await Promise.resolve(
    config({ command: "build", mode: "production", isSsrBuild: false, isPreview: false }),
  );
  const help = (resolved.plugins ?? [])
    .flat()
    .find(
      (plugin) =>
        typeof plugin === "object" &&
        plugin !== null &&
        "name" in plugin &&
        plugin.name === "orpheum-agent-help",
    );
  if (typeof help !== "object" || help === null) {
    throw new Error("the build has no orpheum-agent-help plugin");
  }
  return help as unknown as Record<string, unknown>;
}

/** A plugin's hook, called with whatever the hook is given. */
type Hook = (...given: unknown[]) => unknown;

/** A plugin's hook as a function, whether given bare or as its handler. */
function hookOf(plugin: Record<string, unknown>, name: string): Hook {
  const hook = plugin[name];
  if (typeof hook === "function") {
    return hook as Hook;
  }
  if (
    typeof hook === "object" &&
    hook !== null &&
    "handler" in hook &&
    typeof hook.handler === "function"
  ) {
    return hook.handler as Hook;
  }
  throw new Error(`the plugin has no ${name}`);
}

/** What the build writes from the help's source, by the plugin vite.config.ts builds with. */
async function builtFiles(): Promise<Record<string, string>> {
  const generate = hookOf(await helpPlugin(), "generateBundle");
  const written: Record<string, string> = {};
  const context = {
    emitFile: (file: { fileName?: string; source?: unknown }) => {
      written[file.fileName ?? ""] = String(file.source);
      return file.fileName ?? "";
    },
  };
  await Reflect.apply(generate, context, [{}, {}, false]);
  return written;
}

afterEach(() => {
  vi.unstubAllEnvs();
});

describe("the setup help for a person's AI agent, as built", () => {
  it("is written from its one source, docs/users/agent-setup.md, with /llms.txt naming it", async () => {
    const written = await builtFiles();
    expect(written).toEqual({
      [agentHelpFile]: servedAgentHelp(helpSource),
      [llmsTxtFile]: llmsTxtOf(givenBuild),
    });
    expect(llmsTxt).toContain(`](/${agentHelpFile})`);
  });
  it("points its relative links into the public repository, the rest kept", () => {
    expect(
      servedAgentHelp(
        "[a](install.md#synology) [b](setup-problems.md) [c](../../deploy/catalogue/README.md) " +
          "[d](#your-rules) [e](https://docs.nvidia.com/x)",
      ),
    ).toBe(
      "[a](https://github.com/kstenerud/orpheum/blob/main/docs/users/install.md#synology) " +
        "[b](https://github.com/kstenerud/orpheum/blob/main/docs/users/setup-problems.md) " +
        "[c](https://github.com/kstenerud/orpheum/blob/main/deploy/catalogue/README.md) " +
        "[d](#your-rules) [e](https://docs.nvidia.com/x)",
    );
    expect(servedAgentHelp(helpSource)).not.toMatch(/\]\((?!https:\/\/|#)/u);
  });
});

// An agent's fetch tool handed it a copy of the site an hour older than the one served, and nothing
// in it said so (an agent's install, 2026-11-20).
describe("the build the client names", () => {
  it("is in the page's head, as the build was given it", async () => {
    const transform = hookOf(await helpPlugin(), "transformIndexHtml");
    expect(await Promise.resolve(transform("<html></html>", {}))).toEqual([
      { tag: "meta", attrs: { name: "orpheum-build", content: givenBuild }, injectTo: "head" },
    ]);
    expect(buildMetaName).toBe("orpheum-build");
  });

  it("ends /llms.txt, and nothing is added where no build is named", () => {
    expect(llmsTxtOf(givenBuild)).toBe(
      `${llmsTxt}\nThis copy is from Orpheum build ${givenBuild}.\n`,
    );
    expect(llmsTxtOf(undefined)).toBe(llmsTxt);
  });

  it("is the given build, else what git describes, else none, and never markup", () => {
    const unasked = (): string => {
      throw new Error("git was asked though the build was given");
    };
    expect(buildOf(givenBuild, unasked)).toBe(givenBuild);
    expect(buildOf(undefined, () => "v0.8.7-3-gabc1234\n")).toBe("v0.8.7-3-gabc1234");
    expect(buildOf("", () => "v0.8.7")).toBe("v0.8.7");
    expect(buildOf(undefined, () => undefined)).toBeUndefined();
    expect(() => buildOf('v1"><script>', () => undefined)).toThrow(/not a version or a commit/u);
  });
});

/** Text with each run of spaces and line breaks made one space, as a reader of Markdown takes it. */
const flowed = (text: string): string => text.replaceAll(/\s+/gu, " ");

// An agent's fetch tools summarized the help and dropped its links; only /llms.txt's absolute link
// and the raw copies on GitHub reached it whole (an agent's install, 2026-11-19).
describe("the facts an agent needs first", () => {
  const raw = "https://raw.githubusercontent.com/kstenerud/orpheum/main/docs/users/";
  const releases = "https://github.com/kstenerud/orpheum/releases/latest/download/";

  it("name the install guide and its exact text, the three compose files and the agent's two rules", () => {
    for (const fact of [
      "self-hosted media server",
      "64-bit machine, amd64 or arm64",
      installGuideOnGitHub,
      `${raw}install.md`,
      `${releases}orpheum-omv.yaml`,
      `${releases}orpheum-synology.yaml`,
      `${releases}orpheum-docker.yaml`,
      "Never ask for a password",
      "An invitation link is a secret",
    ]) {
      expect(installFacts).toContain(fact);
    }
  });

  it("open /llms.txt, after its name and summary and before its links", () => {
    const facts = llmsTxt.indexOf(installFacts);
    expect(facts).toBeGreaterThan(llmsTxt.indexOf("> A self-hosted server"));
    expect(facts).toBeLessThan(llmsTxt.indexOf("]("));
  });

  it("open the help, word for word, before its rules", () => {
    const help = flowed(helpSource);
    const facts = help.indexOf(flowed(installFacts).trim());
    expect(facts).toBeGreaterThan(0);
    expect(facts).toBeLessThan(help.indexOf("## Your rules"));
    expect(facts).toBeLessThan(help.indexOf("]("));
  });
});

describe("/llms.txt's guides", () => {
  it("each have their exact text's address beside their github.com page", () => {
    for (const guide of ["agent-setup.md", "install.md", "setup-problems.md"]) {
      expect(llmsTxt).toContain(
        `(https://github.com/kstenerud/orpheum/blob/main/docs/users/${guide})`,
      );
      expect(llmsTxt).toContain(
        `https://raw.githubusercontent.com/kstenerud/orpheum/main/docs/users/${guide}`,
      );
    }
    expect(llmsTxt).toContain("exact text");
  });
});
