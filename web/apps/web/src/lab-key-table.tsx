// SPDX-License-Identifier: MPL-2.0

/**
 * The button test's key table and its context-menu probe (D407 §10): each key with its modifiers,
 * place and composing, read for the volume keys' rule on each layout, and each context menu the
 * page is asked for, which a TV's Menu key may be (plans/player-overlay.md §4).
 */

import "./lab-words.ts";

import { useEffect, type JSX } from "react";
import { useTranslation } from "react-i18next";

import type { NotedEvent } from "./lab-events.ts";
import { yesNo } from "./lab-frame.tsx";
import { fullscreenElementOf } from "./lab-fullscreen.tsx";
import { modifiersText, type KeyEntry } from "./lab-keys.ts";

/**
 * Sends each context menu the page is asked for, by a pointer, a keyboard's Menu key or a remote's,
 * with whether the page was full screen; the browser's own menu is left to open, as it would.
 */
export function useContextMenus(noteEvent: (event: NotedEvent) => void): void {
  useEffect(() => {
    const asked = (): void => {
      noteEvent({
        kind: "contextmenu",
        atMs: Math.round(performance.now()),
        ...(fullscreenElementOf() === null ? {} : { inFullscreen: true }),
      });
    };
    globalThis.addEventListener("contextmenu", asked, true);
    return () => {
      globalThis.removeEventListener("contextmenu", asked, true);
    };
  }, [noteEvent]);
}

/** The keys heard, newest first, each with its modifiers, its place and whether it came composing. */
export function KeyTable(props: { readonly keys: readonly KeyEntry[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="lab__scroll">
      <table className="lab__table">
        <thead>
          <tr>
            <th scope="col">{t("lab.input.keyType")}</th>
            <th scope="col">{t("lab.input.key")}</th>
            <th scope="col">{t("lab.input.code")}</th>
            <th scope="col">{t("lab.input.keyCode")}</th>
            <th scope="col">{t("lab.input.modifiers")}</th>
            <th scope="col">{t("lab.input.location")}</th>
            <th scope="col">{t("lab.input.composing")}</th>
            <th scope="col">{t("lab.input.repeat")}</th>
            <th scope="col">{t("lab.input.since")}</th>
          </tr>
        </thead>
        <tbody>
          {props.keys.map((key, index) => (
            // eslint-disable-next-line @eslint-react/no-array-index-key -- the log's entries have no identity but their place, newest first
            <tr key={index} className={key.isBack ? "lab__back-key" : undefined}>
              <td>{key.type}</td>
              <td>{key.key}</td>
              <td>{key.code}</td>
              <td>{key.keyCode}</td>
              <td>{modifiersText(key.flags)}</td>
              <td>{key.flags.location}</td>
              <td>{yesNo(t, key.flags.isComposing)}</td>
              <td>{yesNo(t, key.isRepeat)}</td>
              <td>{key.sincePreviousMs ?? ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
