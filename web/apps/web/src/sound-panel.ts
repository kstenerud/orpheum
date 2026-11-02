// SPDX-License-Identifier: MPL-2.0

/**
 * The Play page's sound controls as one panel a cast's remote reads and works (playback-sound.ts in
 * the SDK): the dialogue boost's switch, levelling's and the audio track's menu each say here how they
 * stand, which the receiver tells its session for the remote to show, and each carries out the
 * remote's choice of it as its own press, kept for the person as a press is. A Fire TV's own controls
 * are hidden while it is controlled, so its sound could only be changed after taking control back
 * (the owner, VQ116's run).
 */

import type { PlaybackSound, SoundChoice, SoundTrack } from "@orpheum/sdk";
import { createContext, use, useEffect, useRef } from "react";

/** One control's part of the sound, as it stands. */
type SoundPart =
  | { readonly kind: "boost"; readonly isOn: boolean }
  | { readonly kind: "levelling"; readonly isOn: boolean }
  | {
      readonly kind: "audioTrack";
      readonly streamId: number | undefined;
      readonly tracks: readonly SoundTrack[];
    };

/** The page's sound controls, as one panel. */
export interface SoundPanel {
  /** How the film sounds now, as its controls stand; undefined while none is drawn. */
  readonly read: () => PlaybackSound | undefined;
  /** Calls listener at each change of how the controls stand; returns the unsubscribing. */
  readonly subscribe: (listener: () => void) => () => void;
  /** Hands a remote's choice to the control it is of. */
  readonly choose: (choice: SoundChoice) => void;
}

/** What a control does with the panel: says how it stands, and hears the remote's choices of it. */
interface SoundPanelControls {
  readonly show: (part: SoundPart) => void;
  readonly hide: (kind: SoundPart["kind"]) => void;
  readonly hear: (kind: SoundPart["kind"], listener: (choice: SoundChoice) => void) => () => void;
}

/** A panel with no control drawn yet. */
export function newSoundPanel(): SoundPanel & SoundPanelControls {
  const parts = new Map<SoundPart["kind"], SoundPart>();
  const listeners = new Set<() => void>();
  const choosers = new Map<SoundPart["kind"], Set<(choice: SoundChoice) => void>>();
  const changed = (): void => {
    for (const listener of listeners) {
      listener();
    }
  };
  return {
    read: () => {
      if (parts.size === 0) {
        return undefined;
      }
      const boost = parts.get("boost");
      const levelling = parts.get("levelling");
      const audio = parts.get("audioTrack");
      return {
        isBoostOn: boost?.kind === "boost" ? boost.isOn : undefined,
        isLevellingOn: levelling?.kind === "levelling" && levelling.isOn,
        audioStreamId: audio?.kind === "audioTrack" ? audio.streamId : undefined,
        audioTracks: audio?.kind === "audioTrack" ? audio.tracks : [],
      };
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    choose: (choice) => {
      for (const listener of choosers.get(choice.kind) ?? []) {
        listener(choice);
      }
    },
    show: (part) => {
      parts.set(part.kind, part);
      changed();
    },
    hide: (kind) => {
      if (parts.delete(kind)) {
        changed();
      }
    },
    hear: (kind, listener) => {
      const each = choosers.get(kind) ?? new Set();
      choosers.set(kind, each);
      each.add(listener);
      return () => each.delete(listener);
    },
  };
}

/** The Play page's panel, for its controls; undefined on a page with none. */
export const SoundPanelContext = createContext<(SoundPanel & SoundPanelControls) | undefined>(
  undefined,
);

/**
 * Has one control of the page's sound panel say how it stands, part, while it is drawn or offered,
 * and carry out the remote's choice of it by onChosen, as its own press. An undefined part is a
 * control not offered, which the panel does not show.
 */
export function useSoundPart(
  part: SoundPart | undefined,
  onChosen: (choice: SoundChoice) => void,
): void {
  const panel = use(SoundPanelContext);
  const chosenRef = useRef(onChosen);
  useEffect(() => {
    chosenRef.current = onChosen;
  });
  const kind = part?.kind;
  const isOn = part !== undefined && "isOn" in part ? part.isOn : undefined;
  const streamId = part?.kind === "audioTrack" ? part.streamId : undefined;
  const tracks = part?.kind === "audioTrack" ? part.tracks : undefined;
  useEffect(() => {
    if (kind !== undefined) {
      panel?.show(partOf(kind, { isOn, streamId, tracks }));
    }
  }, [panel, kind, isOn, streamId, tracks]);
  useEffect(() => {
    if (panel === undefined || kind === undefined) {
      return undefined;
    }
    const unhear = panel.hear(kind, (choice) => {
      chosenRef.current(choice);
    });
    return () => {
      unhear();
      panel.hide(kind);
    };
  }, [panel, kind]);
}

/** A part from its fields, as useSoundPart keeps them apart for its effect's dependencies. */
function partOf(
  kind: SoundPart["kind"],
  fields: {
    readonly isOn: boolean | undefined;
    readonly streamId: number | undefined;
    readonly tracks: readonly SoundTrack[] | undefined;
  },
): SoundPart {
  return kind === "audioTrack"
    ? { kind, streamId: fields.streamId, tracks: fields.tracks ?? [] }
    : { kind, isOn: fields.isOn === true };
}
