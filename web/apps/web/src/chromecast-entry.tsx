// SPDX-License-Identifier: MPL-2.0

/**
 * Chromecast in Cast's menu (D465 §3): the entry that opens Chrome's own picker, the Google Cast
 * sender it needs, and the start of a cast to the device picked there, said on the film's page as
 * being connected (D570), its failure there too once the menu has closed at the pick.
 */

import "./cast-words.ts";

import type { Api } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { endCastRetrying, type Cast, type CastFilm } from "./cast.ts";
import { loadCastSender } from "./cast-sdk.ts";
import { useShowsDebugTools } from "./debug-tools.tsx";
import {
  castToAGoogleCastDevice,
  endCastSessionOf,
  guestCastOf,
  isCastLogTicked,
  keepCastLogTicked,
  type GuestCastWays,
} from "./google-cast.ts";
import { startChromecastConnecting, type ConnectingTold } from "./google-cast-connecting.ts";
import { FindDevicesIcon } from "./icons.tsx";
import { showNotice } from "./notice.ts";
import { useServices, type AskGuestCastGrant } from "./services.tsx";
import type { RequestState } from "./use-request.ts";

/**
 * What this page casts to a Google Cast device with, where the Cast menu offers Chromecast
 * (guestCastOf); Google's sender script is loaded then, at the menu's first opening, and not before
 * (D465 §3: the owner, 2026-11-12), so no page loads it until someone means to cast. Its grant is
 * the person's (useGuestCastAskingBy for another's).
 */
export function useGuestCast(): GuestCastWays | undefined {
  return useGuestCastAskingBy(useServices().askGuestCastGrant);
}

/**
 * What this page casts to a Google Cast device with, as useGuestCast, its grant asked for by
 * askGrant: a Watch Together guest's by their membership (D518), or none, where none is offered.
 */
export function useGuestCastAskingBy(
  askGrant: AskGuestCastGrant | undefined,
): GuestCastWays | undefined {
  const services = useServices();
  const guest = guestCastOf(
    { ...services, askGuestCastGrant: askGrant },
    globalThis.location.origin,
  );
  const isOffered = guest !== undefined;
  useEffect(() => {
    if (isOffered) {
      loadCastSender().catch(() => undefined); // loaded again at the press, which says its failure
    }
  }, [isOffered]);
  return guest;
}

/**
 * Show cast log, ticked or not in this browser (isTicked, kept by google-cast.ts), and
 * whether a Chromecast cast to now shows its log (showsLog): ticked while the person shows debug
 * tools, the one place the box is offered (the owner, 2026-11-12); off, it has no effect.
 */
interface CastLogChoice {
  readonly isOffered: boolean;
  readonly isTicked: boolean;
  readonly setTicked: (isTicked: boolean) => void;
  readonly showsLog: boolean;
}

/** Show cast log, as this browser keeps it, and whether it has effect now. */
function useCastLogChoice(): CastLogChoice {
  const isOffered = useShowsDebugTools();
  const [isTicked, setIsTicked] = useState(() => isCastLogTicked());
  const setTicked = (isNowTicked: boolean): void => {
    keepCastLogTicked(isNowTicked);
    setIsTicked(isNowTicked);
  };
  return { isOffered, isTicked, setTicked, showsLog: isOffered && isTicked };
}

/**
 * After the person's devices, a separator, then a magnifying glass and Chromecast (D465 §3), which
 * opens Chrome's own picker of the Google Cast devices on the network; none is offered while the live
 * connection is down, as no device is. While the person shows debug tools, Show cast log follows it
 * (CastLogChoice); onCast is told whether the device shows its log.
 */
export function ChromecastEntry(props: {
  readonly isStale: boolean;
  readonly onCast: (showsLog: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const castLog = useCastLogChoice();
  return (
    <>
      <hr className="cast-to__separator" />
      <button
        type="button"
        disabled={props.isStale}
        onClick={() => {
          props.onCast(castLog.showsLog);
        }}
      >
        <FindDevicesIcon />
        {t("cast.chromecast")}
      </button>
      {castLog.isOffered ? (
        <label>
          <input
            type="checkbox"
            checked={castLog.isTicked}
            onChange={(event) => {
              castLog.setTicked(event.target.checked);
            }}
          />{" "}
          {t("cast.showCastLog")}
        </label>
      ) : null}
    </>
  );
}

/** What startsCastsToChromecast starts its casts with, and tells as they go. */
interface ChromecastStarts {
  readonly api: Api;
  readonly locator: () => string | undefined;
  readonly film: () => CastFilm | undefined;
  readonly runRequest: RequestState["runRequest"];
  readonly told: {
    readonly onStarting: () => void;
    /** Told the device's name at the pick in Chrome's picker. */
    readonly onPicked: (name: string) => void;
    readonly onAsking: (asking: { readonly cast: Cast; readonly name: string }) => void;
    /** Told at the pick too, where Cast's menu closes then: a failure is then a notice. */
    readonly onChromecastPicked?: (() => void) | undefined;
  };
  /** A cast's failure in words, as Cast's menu says it. */
  readonly failedText: (cause: unknown) => string;
}

/**
 * Casts to the Google Cast device the person picks in Chrome's picker (castToAGoogleCastDevice). A
 * device picked is said connecting (startChromecastConnecting, D570) until its cast is taken; its
 * Cancel gives the cast up wherever it stands (D570 §3): the session ended, no cast made, or the one
 * made ended as Stop casting ends it. A failure after the pick, Cast's menu closed by then, is said
 * on the page in the menu's words (showNotice).
 */
export function startsCastsToChromecast(
  starts: ChromecastStarts,
): (ways: GuestCastWays, showsLog: boolean) => void {
  const { api, told } = starts;
  return (ways, showsLog) => {
    told.onStarting();
    const abandon = new AbortController();
    let connecting: ConnectingTold | undefined;
    let madeCastId: number | undefined;
    const giveUp = (): void => {
      if (madeCastId !== undefined) {
        endCastSessionOf(madeCastId); // unlinked first, so its end is no Stop from Chrome
        // Failing twice, the core ends it itself once its receiver is gone.
        endCastRetrying(api, madeCastId, "stopped").catch(() => undefined);
      }
      abandon.abort();
    };
    const onPicked = (name: string): void => {
      told.onPicked(name);
      connecting = startChromecastConnecting(name, giveUp);
      told.onChromecastPicked?.();
    };
    starts.runRequest(async () => {
      const how = { locator: starts.locator(), film: starts.film, showsLog, onPicked };
      try {
        const made = await castToAGoogleCastDevice(api, ways, { ...how, abandon: abandon.signal });
        if (made === undefined) {
          connecting?.end();
          return;
        }
        madeCastId = made.cast.castId;
        connecting?.made(made.cast.castId);
        told.onAsking(made);
      } catch (cause) {
        connecting?.end();
        if (connecting !== undefined && told.onChromecastPicked !== undefined) {
          showNotice(starts.failedText(cause));
        }
        throw cause;
      }
    });
  };
}
