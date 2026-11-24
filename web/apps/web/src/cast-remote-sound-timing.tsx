// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import type { CarriesSoundChoices, FileSoundOffsetNow } from "@orpheum/sdk";
import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { useAsPressed } from "./as-pressed.ts";
import { FileOffsetAction, fileTimingText } from "./file-sound-offset.tsx";
import { KeptOpen } from "./over-video-menu.tsx";
import { ProblemReportDialog, type SuggestedSoundOffset } from "./problem-report.tsx";
import { SoundOffsetInput } from "./sound-delay.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

/**
 * The receiver's file's own timing on the remote (D537; the plan's "The cast remote's harder parts",
 * item 1), under Sound timing beside That device, as the player's "This file" is
 * (file-sound-offset.tsx): as the receiver said it stands, the person's own where they keep one,
 * else every person's; each arrow or a number typed sends the receiver the person's own, which it
 * keeps as its own press. Where the person's own is apart from every person's, the player's one row
 * under it: Make it everyone's for an admin, sent to the receiver to make so; Tell the admin for
 * anyone else, the report opened here by onTellTheAdmin with the person's own suggested.
 */
export function RemoteFileTiming(props: {
  readonly offset: FileSoundOffsetNow;
  readonly connection: CarriesSoundChoices;
  readonly onTellTheAdmin: (ownMs: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const isAdmin = use(IsAdminContext);
  const { offset, connection, onTellTheAdmin } = props;
  const own = offset.ownMs;
  const [ms, press] = useAsPressed(own ?? offset.everyonesMs, (next) => {
    connection.chooseSound({ kind: "fileSoundOffset", ms: next });
  });
  return (
    <>
      <KeptOpen>
        <SoundOffsetInput
          label={t("play.soundOffset.thisFile")}
          fieldLabel={t("play.soundOffset.thisFileField")}
          valueText={fileTimingText(t, ms)}
          ms={ms}
          stepLabels={{
            earlier: t("play.soundOffset.thisFileEarlier"),
            later: t("play.soundOffset.thisFileLater"),
          }}
          onChange={press}
        />
      </KeptOpen>
      {own === undefined || own === offset.everyonesMs ? null : (
        <FileOffsetAction
          isAdmin={isAdmin}
          onEveryones={() => {
            connection.chooseSound({ kind: "fileSoundOffset", ms: own, isEveryones: true });
          }}
          onTellTheAdmin={() => {
            onTellTheAdmin(own);
          }}
        />
      )}
    </>
  );
}

/** The report Tell the admin opens on the remote, the receiver's file's offset suggested in it. */
export function RemoteTellTheAdmin(props: {
  readonly suggested: SuggestedSoundOffset;
  readonly onClose: () => void;
}): JSX.Element {
  return <ProblemReportDialog soundOffset={props.suggested} onClose={props.onClose} />;
}
