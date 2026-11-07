// SPDX-License-Identifier: MPL-2.0

import type { CarriesPictureChoices, CastSending, PlaybackPicture } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { playInCast } from "./cast.ts";
import { PlayOriginal } from "./play-original.tsx";
import type { SequenceMoves } from "./player-controls.tsx";
import { useServices } from "./services.tsx";
import { usePrevious } from "./up-next.tsx";

/**
 * What the receiver shows that its person may choose, as it says it, and the remote's way to change
 * it (D440 §5): its subtitles, Off and each track it offers, named as its own menu names them, and
 * the original, offered as the player offers it (a debug tool, D418 §4), each choice sent to the
 * receiver in the cast's session (D434), which carries it out as its own press and says what it
 * shows after. A Fire TV's own menus are hidden while it is controlled (D407 §12). Nothing while
 * the receiver has said nothing.
 */
export function ReceiverPicture(props: {
  readonly picture: PlaybackPicture | undefined;
  readonly connection: CarriesPictureChoices | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { picture, connection } = props;
  if (picture === undefined || connection === undefined) {
    return null;
  }
  return (
    <div className="cast-remote__picture">
      {picture.subtitles.length === 0 ? null : (
        <label className="subtitles">
          {t("play.subtitles.label")}{" "}
          <select
            key={`subtitles-${picture.subtitleKey ?? ""}`}
            defaultValue={picture.subtitleKey ?? ""}
            onChange={(event) => {
              const key = event.currentTarget.value;
              connection.choosePicture({ kind: "subtitles", key: key === "" ? undefined : key });
            }}
          >
            <option value="">{t("play.subtitles.off")}</option>
            {picture.subtitles.map((option) => (
              <option key={option.key} value={option.key}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
      )}
      {picture.canPlayOriginal ? (
        <PlayOriginal
          onChosen={() => {
            connection.choosePicture({ kind: "original" });
          }}
        />
      ) : null}
    </div>
  );
}

/**
 * The remote's ⏮ (D440 §5), as the player's (D407 §2): the episode before the cast's film, which the
 * receiver is handed to play (D364 §3); none, and no button, where nothing plays before it, as on
 * the player (the plan's "restart or previous" names no restart the player has).
 */
export function useRemotePrevious(sent: CastSending): SequenceMoves {
  const { api } = useServices();
  const previous = usePrevious(sent.fileId);
  if (previous === undefined || previous.unavailable !== undefined) {
    return {};
  }
  const { fileId } = previous;
  return {
    previous: () => {
      playInCast(api, sent.castId, { fileId }).catch(() => undefined); // the cast goes on as it was
    },
  };
}
