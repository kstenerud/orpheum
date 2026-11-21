// SPDX-License-Identifier: MPL-2.0

/**
 * The setup help for a person's own AI agent, which the page names to an agent that reads it
 * without running its script (index.html's head and its root's paragraph): the build writes it
 * beside the client's files from its one source, docs/users/agent-setup.md (vite.config.ts), with
 * /llms.txt pointing to it, so the core and the loader serve both as their other files (the
 * plan's "Help for a person's own AI agent", navigation-and-widgets-answers-2026-10-27.md).
 */

/** The page's build marker, which the build writes into index.html's head (vite.config.ts). */
export { buildMetaName } from "./page-build.ts";

/** The help's one source, from the repository's root. */
export const agentHelpSource = "docs/users/agent-setup.md";

/** Where the built client has the help, and the page's links name it: `/agent-setup.md`. */
export const agentHelpFile = "agent-setup.md";

/** The file some agents look for at a site's root, naming the help (llmstxt.org). */
export const llmsTxtFile = "llms.txt";

/** The public repository's tree of the user documentation, which the help's links point into. */
const usersGuideOnGitHub = "https://github.com/kstenerud/orpheum/blob/main/docs/users/";

/**
 * The same tree as plain files: a fetch of one gives its exact text, where a fetch of the GitHub page
 * may give an agent a summary without its links (an agent's install, 2026-11-19).
 */
const usersGuideRaw = "https://raw.githubusercontent.com/kstenerud/orpheum/main/docs/users/";

/** The install guide in the public repository, which /llms.txt and the realm sign-in link to. */
export const installGuideOnGitHub: "https://github.com/kstenerud/orpheum/blob/main/docs/users/install.md" = `${usersGuideOnGitHub}install.md`;

/** The latest release's compose files, one for each way to install (install.md, One compose file). */
const composeFiles = "https://github.com/kstenerud/orpheum/releases/latest/download/";

/** What Orpheum is, in the sentence /llms.txt and the page's description (index.html) both say. */
export const orpheumInOneSentence =
  "A self-hosted server that plays a household's own films and series to its browsers, phones and televisions.";

/**
 * The facts an agent needs first, which open /llms.txt and the help (docs/users/agent-setup.md has
 * them word for word): short and plain, with each address written out, so a fetch that summarizes
 * the file or drops its links keeps them (an agent's install, 2026-11-19).
 */
export const installFacts: "- Orpheum is a self-hosted media server. It runs in Docker (Docker Engine and its compose plugin) on a 64-bit machine, amd64 or arm64.\n- Install guide: https://github.com/kstenerud/orpheum/blob/main/docs/users/install.md\n- Its exact text: https://raw.githubusercontent.com/kstenerud/orpheum/main/docs/users/install.md\n- Compose file for OpenMediaVault: https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-omv.yaml\n- Compose file for a Synology: https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-synology.yaml\n- Compose file for any other machine with Docker: https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-docker.yaml\n- Never ask for a password: the person types it themselves.\n- An invitation link is a secret: never paste one into a chat, a document or an issue that anyone else can read.\n" = `- Orpheum is a self-hosted media server. It runs in Docker (Docker Engine and its compose plugin) on a 64-bit machine, amd64 or arm64.
- Install guide: ${installGuideOnGitHub}
- Its exact text: ${usersGuideRaw}install.md
- Compose file for OpenMediaVault: ${composeFiles}orpheum-omv.yaml
- Compose file for a Synology: ${composeFiles}orpheum-synology.yaml
- Compose file for any other machine with Docker: ${composeFiles}orpheum-docker.yaml
- Never ask for a password: the person types it themselves.
- An invitation link is a secret: never paste one into a chat, a document or an issue that anyone else can read.
`;

/** A Markdown link's target: what is between `](` and `)`. */
const linkTarget = /\]\(([^)\s]+)\)/gu;

/**
 * The help as served: its source with every relative link made the public repository's, since a
 * link relative to the served file would name a file the client does not have. A link to a heading
 * of the help itself, or to elsewhere, is kept.
 */
export function servedAgentHelp(source: string): string {
  return source.replaceAll(linkTarget, (link, target: string) =>
    target.startsWith("#") || /^[a-z][a-z0-9+.-]*:/iu.test(target)
      ? link
      : `](${new URL(target, usersGuideOnGitHub).href})`,
  );
}

/**
 * /llms.txt: the site named, the facts an agent needs first, then the help, by the address the page
 * names it at, and each guide with its exact text beside it.
 */
export const llmsTxt: "# Orpheum\n\n> A self-hosted server that plays a household's own films and series to its browsers, phones and televisions. An AI agent helping a person set it up starts with the\n> facts and the setup help below.\n\n- Orpheum is a self-hosted media server. It runs in Docker (Docker Engine and its compose plugin) on a 64-bit machine, amd64 or arm64.\n- Install guide: https://github.com/kstenerud/orpheum/blob/main/docs/users/install.md\n- Its exact text: https://raw.githubusercontent.com/kstenerud/orpheum/main/docs/users/install.md\n- Compose file for OpenMediaVault: https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-omv.yaml\n- Compose file for a Synology: https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-synology.yaml\n- Compose file for any other machine with Docker: https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-docker.yaml\n- Never ask for a password: the person types it themselves.\n- An invitation link is a secret: never paste one into a chat, a document or an issue that anyone else can read.\n\n## Setting up\n\nEach guide's raw.githubusercontent.com address gives its exact text, where its github.com page may\nreach an agent only as a summary.\n\n- [Setup help for an AI agent](/agent-setup.md): what to ask the person, which section of the\n  install guide each answer leads to, and the agent's rules.\n- [The same help in the public repository](https://github.com/kstenerud/orpheum/blob/main/docs/users/agent-setup.md); its exact text:\n  https://raw.githubusercontent.com/kstenerud/orpheum/main/docs/users/agent-setup.md\n- [The install guide](https://github.com/kstenerud/orpheum/blob/main/docs/users/install.md); its exact text: https://raw.githubusercontent.com/kstenerud/orpheum/main/docs/users/install.md\n- [What has gone wrong in an install before, and the fix](https://github.com/kstenerud/orpheum/blob/main/docs/users/setup-problems.md); its\n  exact text: https://raw.githubusercontent.com/kstenerud/orpheum/main/docs/users/setup-problems.md\n" = `# Orpheum

> ${orpheumInOneSentence} An AI agent helping a person set it up starts with the
> facts and the setup help below.

${installFacts}
## Setting up

Each guide's raw.githubusercontent.com address gives its exact text, where its github.com page may
reach an agent only as a summary.

- [Setup help for an AI agent](/${agentHelpFile}): what to ask the person, which section of the
  install guide each answer leads to, and the agent's rules.
- [The same help in the public repository](${usersGuideOnGitHub}agent-setup.md); its exact text:
  ${usersGuideRaw}agent-setup.md
- [The install guide](${installGuideOnGitHub}); its exact text: ${usersGuideRaw}install.md
- [What has gone wrong in an install before, and the fix](${usersGuideOnGitHub}setup-problems.md); its
  exact text: ${usersGuideRaw}setup-problems.md
`;

/** A build's name as the marker takes it: a version, a commit or both, never markup. */
const buildName = /^[\w.+-]+$/u;

/** /llms.txt, ending with the build it is from when the build is named. */
export function llmsTxtOf(build: string | undefined): string {
  return build === undefined ? llmsTxt : `${llmsTxt}\nThis copy is from Orpheum build ${build}.\n`;
}

/**
 * The build the client is built as: ORPHEUM_BUILD where the build is given it (orpheum-connect's
 * package, built from a copy without git's history), else what describe names, the version tag
 * and any commits after it; undefined when neither names one.
 */
export function buildOf(
  given: string | undefined,
  describe: () => string | undefined,
): string | undefined {
  const build = given === undefined || given === "" ? describe()?.trim() : given;
  if (build === undefined || build === "") {
    return undefined;
  }
  if (!buildName.test(build)) {
    throw new Error(`the build's name ${JSON.stringify(build)} is not a version or a commit`);
  }
  return build;
}

/** One file the build writes beside the client's. */
export interface AgentHelpFile {
  readonly fileName: string;
  readonly source: string;
}

/** The files the build writes from the help's source: the help as served, and /llms.txt naming build. */
export function agentHelpFiles(
  source: string,
  build: string | undefined,
): readonly AgentHelpFile[] {
  return [
    { fileName: agentHelpFile, source: servedAgentHelp(source) },
    { fileName: llmsTxtFile, source: llmsTxtOf(build) },
  ];
}
