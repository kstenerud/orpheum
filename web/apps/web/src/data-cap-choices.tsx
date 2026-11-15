// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import { offeredRungsOf, playerOfferedRungsEvent } from "@orpheum/player";
import { useEffect, useEffectEvent, useSyncExternalStore, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { deviceDataCaps, gbPerHourOf, rungUnderCap, type DataCaps } from "./data-cap.ts";
import { ListNote, ListOption } from "./list-box.tsx";

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
 * ⁝'s data use (D488): a line naming the cap, for the network the device is on where its browser
 * says which, then No limit and one choice per rung the core offers the film, highest first, each
 * its video's and sound's GB per hour, the one the cap plays marked. Nothing while the film plays
 * its own file, which offers no rungs: a cap is chosen on a film on its ladder.
 */
export function DataCapChoices(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly caps?: DataCaps | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const caps = props.caps ?? deviceDataCaps();
  const rungs = useOfferedRungs(props.videoRef);
  const { kind, capKbps } = useCapNow(caps);
  if (rungs === undefined || rungs.length === 0) {
    return null;
  }
  const playing = rungUnderCap(rungs, capKbps);
  return (
    <>
      <ListNote>{t(`play.dataCap.heading.${kind}`)}</ListNote>
      <ListOption
        isCurrent={playing === undefined}
        onChoose={() => {
          caps.choose(undefined);
        }}
      >
        {t("play.dataCap.noLimit")}
      </ListOption>
      {[...rungs].reverse().map((rung) => (
        <ListOption
          key={rung}
          isCurrent={rung === playing}
          onChoose={() => {
            caps.choose(rung);
          }}
        >
          {t("play.dataCap.perHour", { gb: gbPerHourOf(rung).toFixed(1) })}
        </ListOption>
      ))}
    </>
  );
}

/** Whether this device's cap holds now, which plays no film's own file (D488). */
export function useIsCapped(caps: DataCaps = deviceDataCaps()): boolean {
  return useCapNow(caps).capKbps !== undefined;
}

/**
 * Calls onStart, where given, when this device's cap starts to hold where none did: a film playing
 * its own file then asks for its ladder from where it stands, which a cap plays it on (D489).
 */
export function useWhenACapStarts(onStart: (() => void) | undefined): void {
  const started = useEffectEvent(() => {
    onStart?.();
  });
  useEffect(
    () =>
      deviceDataCaps().whenCapStarts(() => {
        started();
      }),
    [],
  );
}
