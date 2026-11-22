// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import { offeredRungsOf, playerOfferedRungsEvent } from "@orpheum/player";
import {
  useEffect,
  useEffectEvent,
  useSyncExternalStore,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import {
  deviceDataCaps,
  gbPerHourOf,
  isWithinCap,
  rungUnderCap,
  type DataCaps,
} from "./data-cap.ts";
import { ListOption, ListSubmenu } from "./list-box.tsx";

/**
 * The rungs of the film the video in videoRef plays on its ladder (offeredRungsOf), drawn again as
 * its player tells them; undefined while it plays no ladder, its own file.
 */
function useOfferedRungs(
  videoRef: RefObject<HTMLVideoElement | null>,
): readonly number[] | undefined {
  return useSyncExternalStore(
    (told) => {
      const video = videoRef.current;
      video?.addEventListener(playerOfferedRungsEvent, told);
      return () => {
        video?.removeEventListener(playerOfferedRungsEvent, told);
      };
    },
    () => {
      const video = videoRef.current;
      return video === null ? undefined : offeredRungsOf(video);
    },
  );
}

/** The cap now and the network it is for, drawn again at each choice and change of network. */
function useCapNow(caps: DataCaps): {
  readonly kind: string;
  readonly capKbps: number | undefined;
} {
  const now = useSyncExternalStore(
    (listener) => caps.subscribe(listener),
    () => `${caps.kind()} ${String(caps.capKbps() ?? "")}`,
  );
  const [kind = "unknown", kept = ""] = now.split(" ");
  return { kind, capKbps: kept === "" ? undefined : Number(kept) };
}

/**
 * ⁝'s Quality (D538 §10), a submenu (D490) of the device's data use (D488): its entry names the cap,
 * for the network the device is on where its browser says which, and what it plays, "Quality on
 * Wi-Fi: 0.6 GB per hour ›"; its list Automatic, no cap, then one choice per rung the core offers
 * the film, highest first, each its video's and sound's GB per hour, the film's own picture among
 * them (D491), the one the cap plays marked, then what follows them (children): the original file,
 * a debug tool chosen for one playback. A cap chosen is kept for the network and the films after,
 * as Data use's always was. Nothing while the film plays its own file, which offers no rungs: a cap
 * is chosen on a film on its ladder.
 */
export function QualityChoices(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly caps?: DataCaps | undefined;
  /** What follows the rungs: the original's entry, where the page offers it. */
  readonly children?: ReactNode;
}): JSX.Element | null {
  const { t } = useTranslation();
  const caps = props.caps ?? deviceDataCaps();
  const rungs = useOfferedRungs(props.videoRef);
  const { kind, capKbps } = useCapNow(caps);
  if (rungs === undefined || rungs.length === 0) {
    return null;
  }
  const playing = rungUnderCap(rungs, capKbps);
  const perHourOf = (rung: number): string =>
    t("play.quality.perHour", { gb: gbPerHourOf(rung).toFixed(1) });
  return (
    <ListSubmenu
      name={t(`play.quality.heading.${kind}`)}
      value={playing === undefined ? t("play.quality.automatic") : perHourOf(playing)}
    >
      <ListOption
        isCurrent={playing === undefined}
        onChoose={() => {
          caps.choose(undefined);
        }}
      >
        {t("play.quality.automatic")}
      </ListOption>
      {[...rungs].reverse().map((rung) => (
        <ListOption
          key={rung}
          isCurrent={rung === playing}
          onChoose={() => {
            caps.choose(rung);
          }}
        >
          {perHourOf(rung)}
        </ListOption>
      ))}
      {props.children}
    </ListSubmenu>
  );
}

/**
 * Whether this device's cap now lets a film's own file of fileKbps play (isWithinCap): the original is
 * offered only then (D488 §4, D491).
 */
export function useIsWithinCap(fileKbps: number, caps: DataCaps = deviceDataCaps()): boolean {
  return isWithinCap(fileKbps, useCapNow(caps).capKbps);
}

/**
 * Calls onFall, where given, when this device's cap falls under fileKbps where it was not: a film
 * playing its own file of that rate then asks for its ladder from where it stands, which a cap
 * plays it on (D489, D491).
 */
export function useWhenTheCapFallsUnder(fileKbps: number, onFall: (() => void) | undefined): void {
  const fell = useEffectEvent(() => {
    onFall?.();
  });
  useEffect(
    () =>
      deviceDataCaps().whenCapFallsUnder(fileKbps, () => {
        fell();
      }),
    [fileKbps],
  );
}
