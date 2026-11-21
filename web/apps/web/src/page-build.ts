// SPDX-License-Identifier: MPL-2.0

/**
 * The meta element naming the build the page is from, and /llms.txt's last line naming it too: an
 * agent whose fetch tool handed it a copy older than the site's can tell, and a check after a deploy
 * can see the new build is served (an agent's install, 2026-11-20). The page reads it back to name
 * its build in its service worker's address (serviceWorkerUrlOf). Kept apart from agent-help.ts, so
 * the page's bundle carries none of /llms.txt's words.
 */
export const buildMetaName = "orpheum-build";

/** The build the page is from, as its meta element names it; undefined where none does (the dev loop). */
export function buildOfPage(page: Document): string | undefined {
  const content = page.querySelector(`meta[name="${buildMetaName}"]`)?.getAttribute("content");
  return content === null || content === undefined || content === "" ? undefined : content;
}
