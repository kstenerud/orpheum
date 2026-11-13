// SPDX-License-Identifier: MPL-2.0

import { readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import {
  agentHelpFile,
  agentHelpSource,
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
