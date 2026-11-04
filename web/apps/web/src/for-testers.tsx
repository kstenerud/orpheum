// SPDX-License-Identifier: MPL-2.0

import { createContext, use, useEffect, useRef, useState, type JSX, type ReactNode } from "react";
import { createPortal } from "react-dom";

/**
 * The player's For testers panel (item 9 of ui-audit-2026-11-04.md; D418 §1): the notes and tools a
 * tester reads, which sat under the film until the film filled the window and hid them. They are
 * kept here, drawn and running while the film plays, in the page but hidden (ForTestersHome), and
 * shown when ⁝'s For testers opens its dialog (player-menus.tsx). Undefined outside a player that
 * gives one, or drawn with no document, where they show in place.
 */
export const ForTestersContext = createContext<HTMLElement | undefined>(undefined);

/** Where each panel is kept, hidden, while its dialog is closed (ForTestersHome). */
const homes = new WeakMap<HTMLElement, HTMLElement>();

/** A panel for ForTestersContext, kept by the player for as long as it shows; none with no document. */
export function useForTestersPanel(): HTMLElement | undefined {
  const [panel] = useState(() => {
    if (typeof document === "undefined") {
      return undefined;
    }
    const made = document.createElement("div");
    made.className = "for-testers";
    return made;
  });
  return panel;
}

/** Keeps the player's For testers panel in the page, hidden, while For testers is closed. */
export function ForTestersHome(): JSX.Element | null {
  const panel = use(ForTestersContext);
  const homeRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const home = homeRef.current;
    if (panel === undefined || home === null) {
      return undefined;
    }
    homes.set(panel, home);
    home.append(panel);
    return () => {
      homes.delete(panel);
      panel.remove();
    };
  }, [panel]);
  return panel === undefined ? null : <div ref={homeRef} hidden />;
}

/** Its children in the player's For testers panel where one is given (ForTestersContext), else in place. */
export function ForTesters(props: { readonly children: ReactNode }): JSX.Element {
  const panel = use(ForTestersContext);
  return panel === undefined ? <>{props.children}</> : createPortal(props.children, panel);
}

/** Shows the player's For testers panel here while drawn, putting it back in its hidden home after. */
export function ForTestersShown(): JSX.Element | null {
  const panel = use(ForTestersContext);
  const placeRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const place = placeRef.current;
    if (panel === undefined || place === null) {
      return undefined;
    }
    place.append(panel);
    return () => {
      const home = homes.get(panel);
      if (home === undefined) {
        panel.remove();
      } else {
        home.append(panel);
      }
    };
  }, [panel]);
  return panel === undefined ? null : <div ref={placeRef} />;
}
