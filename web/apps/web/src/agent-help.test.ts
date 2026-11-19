// SPDX-License-Identifier: MPL-2.0

import { readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import {
  agentHelpFile,
  agentHelpSource,
  installFacts,
  installGuideOnGitHub,
  llmsTxt,
  llmsTxtFile,
  servedAgentHelp,
} from "./agent-help.ts";

/** The help's one source, under docs/users/. */
const helpSource = readFileSync(
  path.join(import.meta.dirname, "../../../..", agentHelpSource),
  "utf8",
);

/** What the build writes from the help's source, by the plugin vite.config.ts builds with. */
async function builtFiles(): Promise<Record<string, string>> {
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
  if (typeof help !== "object" || help === null || !("generateBundle" in help)) {
    throw new Error("the build has no orpheum-agent-help plugin");
  }
  const hook = help.generateBundle;
  const generate = typeof hook === "function" ? hook : hook.handler;
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

describe("the setup help for a person's AI agent, as built", () => {
  it("is written from its one source, docs/users/agent-setup.md, with /llms.txt naming it", async () => {
    const written = await builtFiles();
    expect(written).toEqual({
      [agentHelpFile]: servedAgentHelp(helpSource),
      [llmsTxtFile]: llmsTxt,
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
