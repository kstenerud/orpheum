// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, useSyncExternalStore, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { DebugTool } from "./debug-tools.tsx";
import { dropChoicesS, linkCapChoicesKbps, type LinkDebug } from "./link-debug.ts";
import { useServices } from "./services.tsx";

/** A cap as the page says it: megabits a second, as few digits as it needs ("0.5", "1"). */
function capRateText(kbps: number): string {
  return String(kbps / 1000);
}

const noChange = (): (() => void) => () => undefined;
const noCap = (): undefined => undefined;

/** The tab's cap now, followed as it changes; undefined for none, or where nothing shapes the link. */
function useCapKbps(linkDebug: LinkDebug | undefined): number | undefined {
  const read = linkDebug?.capKbps ?? noCap;
  return useSyncExternalStore(linkDebug?.subscribe ?? noChange, read, read);
}

/**
 * The debug link's tools on the player (D380 §1), shown only to a person who shows debug tools: a cap
 * on this tab's download rate, and dropping its connection for 10, 30 or 60 s, which says how long is
 * left while it lasts. Chrome's DevTools throttling never reached the WebRTC carrier (VQ128's run).
 */
export function LinkDebugTools(): JSX.Element {
  return (
    <DebugTool>
      <LinkDebugControls />
    </DebugTool>
  );
}

function LinkDebugControls(): JSX.Element | null {
  const { t } = useTranslation();
  const { linkDebug } = useServices();
  const capKbps = useCapKbps(linkDebug);
  const leftS = useDropLeftS(linkDebug);
  if (linkDebug === undefined) {
    return null;
  }
  return (
    <div className="play__link-debug">
      <label>
        {t("debugTools.linkCap")}{" "}
        <select
          value={capKbps ?? ""}
          onChange={(event) => {
            const chosen = Number(event.currentTarget.value);
            linkDebug.setCap(chosen > 0 ? chosen : undefined);
          }}
        >
          <option value="">{t("debugTools.linkCapOff")}</option>
          {linkCapChoicesKbps.map((kbps) => (
            <option key={kbps} value={kbps}>
              {t("debugTools.linkCapRate", { rate: capRateText(kbps) })}
            </option>
          ))}
        </select>
      </label>
      {dropChoicesS.map((seconds) => (
        <button
          key={seconds}
          type="button"
          disabled={leftS > 0}
          onClick={() => {
            linkDebug.dropFor(seconds * 1000);
          }}
        >
          {t("debugTools.drop", { seconds })}
        </button>
      ))}
      {leftS > 0 ? <output>{t("debugTools.dropped", { seconds: leftS })}</output> : null}
    </div>
  );
}

/** How many whole seconds of the tab's drop are left, rounded up; 0 while there is none. */
function dropLeftS(linkDebug: LinkDebug | undefined): number {
  return linkDebug === undefined ? 0 : Math.ceil(linkDebug.droppedForMs() / 1000);
}

/** How many seconds of the tab's drop are left, counted down each second; 0 while there is none. */
function useDropLeftS(linkDebug: LinkDebug | undefined): number {
  const [leftS, setLeftS] = useState(() => dropLeftS(linkDebug));
  useEffect(() => {
    if (linkDebug === undefined) {
      return undefined;
    }
    const read = (): void => {
      setLeftS(dropLeftS(linkDebug));
    };
    const unsubscribe = linkDebug.subscribe(read);
    const ticking = setInterval(read, 1000);
    return () => {
      unsubscribe();
      clearInterval(ticking);
    };
  }, [linkDebug]);
  return leftS;
}

/**
 * The page's word, above every screen, that this tab's download rate is capped, the whole time it is
 * (D380 §4): a DevTools box left ticked once cost an afternoon (agent-failures A51). Shown whether or
 * not the person shows debug tools, as a cap outlives the screen it was set on.
 */
export function LinkCapBadge(): JSX.Element | null {
  const { t } = useTranslation();
  const { linkDebug } = useServices();
  const capKbps = useCapKbps(linkDebug);
  return capKbps === undefined ? null : (
    <output className="app-shell__note">
      {t("debugTools.throttled", { rate: capRateText(capKbps) })}
    </output>
  );
}
