// SPDX-License-Identifier: MPL-2.0

import "./sound-on-pill.css";
import "./cast-words.ts";

import type { TFunction } from "i18next";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { inThisDevicesWords } from "./cast-start.tsx";
import { ActionPill, Pill } from "./pill.tsx";

/**
 * The one prompt over a film that starts muted (D538 §1; the player spec's "Turning the sound on"),
 * a Watch Together guest's or host's or a cast's receiver's: at the film's top centre, in this
 * device's words ("Tap to turn the sound on", "Press OK to…", "Click to…"); or, while Watch
 * Together's voices wait for a tap, "Tap to hear everyone's voices", which turns the sound on too and
 * so wins where both apply. An action Pill where onPress is given; where any press answers it, a
 * receiver's, a notice that says so.
 */
export function SoundOnPill(props: {
  readonly isForVoices?: boolean | undefined;
  readonly onPress?: (() => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const words = props.isForVoices === true ? t("together.voice.tapToHear") : soundOnWords(t);
  return (
    <div className="sound-on-pill">
      {props.onPress === undefined ? (
        <Pill>{words}</Pill>
      ) : (
        <ActionPill onPress={props.onPress}>{words}</ActionPill>
      )}
    </div>
  );
}

/** The press that turns the sound on, in this device's words for one. */
function soundOnWords(t: TFunction): string {
  return inThisDevicesWords({
    television: t("cast.soundOn.television"),
    touch: t("cast.soundOn.touch"),
    pointer: t("cast.soundOn.pointer"),
  });
}
