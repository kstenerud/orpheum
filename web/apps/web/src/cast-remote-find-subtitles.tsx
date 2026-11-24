// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import type { CarriesPictureChoices } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { ListOption } from "./list-box.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { FindSubtitles } from "./subtitle-search.tsx";

/** 💬's Find subtitles on the remote, as the player's: opens the search by onFind. */
export function RemoteFindSubtitlesOption(props: { readonly onFind: () => void }): JSX.Element {
  const { t } = useTranslation();
  return <ListOption onChoose={props.onFind}>{t("play.findSubtitles.open")}</ListOption>;
}

/**
 * Find subtitles on the remote (the plan's "The cast remote's harder parts", item 3), the player's
 * dialog over the receiver's file: the remote fetches the subtitle for the file, as the player does,
 * then tells the receiver one was fetched, which reads its tracks again as its own Find subtitles
 * does after a fetch (picture-panel.ts).
 */
export function RemoteFindSubtitles(props: {
  readonly fileId: number;
  readonly connection: CarriesPictureChoices;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { fileId, connection, onClose } = props;
  return (
    <ModalDialog label={t("play.findSubtitles.heading")} onClose={onClose}>
      <FindSubtitles
        fileId={fileId}
        isOpenAtOnce
        onFetched={() => {
          onClose();
          connection.choosePicture({ kind: "subtitleFetched" });
        }}
      />
    </ModalDialog>
  );
}
