// SPDX-License-Identifier: MPL-2.0

/**
 * What a client origin shows once the core was reached but its client's script did not load (D308
 * §6): the page the loader had taken down for the client is otherwise left blank. A worker bypassed
 * for the network, as DevTools' "Bypass for network" does, sent the client's script to the
 * discovery, which has none, and the page stayed black, saying nothing (the owner's Mac, staging,
 * 2026-10-29).
 */

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { copyToClipboard } from "./clipboard.ts";
import { CopyWhatHappened } from "./what-happened.tsx";

/**
 * What happened, for its person to copy and send (K16): the server, the file that did not load,
 * whether a service worker controlled the page, and the browser. It names no person and holds no
 * secret.
 */
export function clientFailedText(
  failed: {
    readonly coreId: string;
    readonly address: string;
    readonly isControlled: boolean;
  },
  userAgent: string,
): string {
  return [
    "Orpheum reached its server, but its client did not load.",
    `Server: ${failed.coreId}`,
    `File not loaded: ${failed.address}`,
    `Service worker controlling the page: ${failed.isControlled ? "yes" : "no"}`,
    'A service worker bypassed for the network gives this: with DevTools open, untick "Bypass for network" under Application → Service workers, then reload.',
    `Browser: ${userAgent}`,
  ].join("\n");
}

/**
 * The page saying the client did not load, with what happened to copy; where a worker controls the
 * page, as when DevTools bypass it, how to untick that, which is what a person at the page can do
 * (the owner, 2026-10-30).
 */
export function ClientFailedPage(props: {
  readonly text: string;
  readonly isControlled: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <main>
      <p role="alert">{t("loader.clientFailed")}</p>
      {props.isControlled ? <p>{t("loader.clientFailedBypass")}</p> : null}
      <CopyWhatHappened text={props.text} copy={copyToClipboard} />
    </main>
  );
}
