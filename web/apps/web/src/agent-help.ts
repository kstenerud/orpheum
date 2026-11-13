// SPDX-License-Identifier: MPL-2.0

/**
 * The setup help for a person's own AI agent, which the page names to an agent that reads it
 * without running its script (index.html's head and its root's paragraph): the build writes it
 * beside the client's files from its one source, docs/users/agent-setup.md (vite.config.ts), with
 * /llms.txt pointing to it, so the core and the loader serve both as their other files (the
 * plan's "Help for a person's own AI agent", navigation-and-widgets-answers-2026-10-27.md).
 */

/** The help's one source, from the repository's root. */
export const agentHelpSource = "docs/users/agent-setup.md";

/** Where the built client has the help, and the page's links name it: `/agent-setup.md`. */
export const agentHelpFile = "agent-setup.md";

/** The file some agents look for at a site's root, naming the help (llmstxt.org). */
export const llmsTxtFile = "llms.txt";

/** The public repository's tree of the user documentation, which the help's links point into. */
const usersGuideOnGitHub = "https://github.com/kstenerud/orpheum/blob/main/docs/users/";

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

/** /llms.txt: the site named, and the help listed first, by the address the page names it at. */
export const llmsTxt = `# Orpheum

> A self-hosted server that plays a household's own films and series to its browsers, phones and
> televisions. An AI agent helping a person set it up starts with the setup help below.

## Setting up

- [Setup help for an AI agent](/${agentHelpFile}): what to ask the person, which section of the
  install guide each answer leads to, and the agent's rules.
- [The same help in the public repository](${usersGuideOnGitHub}agent-setup.md)
- [The install guide](${usersGuideOnGitHub}install.md)
`;

/** One file the build writes beside the client's. */
export interface AgentHelpFile {
  readonly fileName: string;
  readonly source: string;
}

/** The files the build writes from the help's source: the help as served, and /llms.txt. */
export function agentHelpFiles(source: string): readonly AgentHelpFile[] {
  return [
    { fileName: agentHelpFile, source: servedAgentHelp(source) },
    { fileName: llmsTxtFile, source: llmsTxt },
  ];
}
