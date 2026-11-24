// SPDX-License-Identifier: MPL-2.0

import type { CarriesPictureChoices } from "@orpheum/sdk";
import type { JSX } from "react";

import { KeptOpen } from "./over-video-menu.tsx";
import { longestSubtitleOffsetMs, OffsetControl } from "./subtitles.tsx";

/**
 * The receiver's subtitle timing on the remote (the plan's "The cast remote's harder parts", item
 * 2), the player's 💬 stepper (OffsetControl) under its tracks: as the receiver said it stands, none
 * said being in step; each step or a number typed sends the receiver the timing it lands on, which
 * the receiver sets as its own stepper's press (picture-panel.ts).
 */
export function RemoteSubtitleTiming(props: {
  readonly offsetMs: number | undefined;
  readonly connection: CarriesPictureChoices;
}): JSX.Element {
  const { connection } = props;
  const offsetMs = props.offsetMs ?? 0;
  return (
    <KeptOpen>
      <OffsetControl
        offsetMs={offsetMs}
        onMove={(stepMs) => {
          const ms = Math.max(
            -longestSubtitleOffsetMs,
            Math.min(longestSubtitleOffsetMs, offsetMs + stepMs),
          );
          connection.choosePicture({ kind: "subtitleOffset", ms });
        }}
      />
    </KeptOpen>
  );
}
