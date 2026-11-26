// SPDX-License-Identifier: MPL-2.0

import type { Api, CastSending, PictureChoice, SoundChoice } from "@orpheum/sdk";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { keepAudioChoice } from "./audio-tracks.tsx";
import type { ReceiverChoices } from "./cast-remote-choices.tsx";
import { useIsGuest } from "./guest-page.ts";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { keepOwnChoice } from "./sound.tsx";
import { keepOffset, keepSubtitleChoice } from "./subtitles.tsx";

/** The receiver's choices as the remote sends them, and why keeping the last failed, if it did. */
export interface KeptChoices {
  readonly connection: ReceiverChoices | undefined;
  /** Why keeping the last choice failed, in the viewer's words; undefined while nothing has. */
  readonly problem: string | undefined;
}

/**
 * The receiver's choices, each one the cast's remote sends to a guest receiver (a Chromecast, D465
 * §1) kept for the person here, as the Play page keeps it (sound.tsx, audio-tracks.tsx,
 * subtitles.tsx): a guest receiver is refused the person's routes (D465 §2), so the sender, signed in
 * as the person, keeps them. A receiver signed in as the person (D469) keeps each choice itself, as
 * its own press would, so the sender keeps none of its: each choice has one keeper, and no two saves
 * of it race. A guest's remote keeps nothing, refused the person's routes as well (D469 §4). Only the
 * person's own choices are kept: the boost and levelling for every file; the audio track, the
 * subtitles and their timing in the receiver's file. The sound's timing on the device is its own.
 */
export function useKeptByTheSender(
  connection: ReceiverChoices | undefined,
  sending: CastSending | undefined,
): KeptChoices {
  const { t } = useTranslation();
  const { api } = useServices();
  const isGuest = useIsGuest();
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const fileId = sending?.fileId ?? 0;
  const isKeptHere = sending?.isGuestReceiver === true && !isGuest && fileId !== 0;
  const kept = useMemo((): ReceiverChoices | undefined => {
    if (connection === undefined || !isKeptHere) {
      return connection;
    }
    const keep = (keeping: Promise<void> | undefined): void => {
      keeping?.then(
        () => {
          setProblem(undefined);
        },
        (cause: unknown) => {
          setProblem(problemText(t, cause));
        },
      );
    };
    return {
      chooseSound: (choice) => {
        connection.chooseSound(choice);
        keep(keptSound(api, fileId, choice));
      },
      subscribeToSoundChoices: (listener) => connection.subscribeToSoundChoices(listener),
      choosePicture: (choice) => {
        connection.choosePicture(choice);
        keep(keptPicture(api, fileId, choice));
      },
      subscribeToPictureChoices: (listener) => connection.subscribeToPictureChoices(listener),
    };
  }, [api, connection, fileId, isKeptHere, t]);
  return { connection: kept, problem };
}

/** Keeps a sound choice the person keeps, as the Play page does; undefined for one they do not. */
function keptSound(api: Api, fileId: number, choice: SoundChoice): Promise<void> | undefined {
  switch (choice.kind) {
    case "boost":
      return keepOwnChoice(api, "isDialogueBoostOn", choice.isOn);
    case "levelling":
      return keepOwnChoice(api, "isLevellingOn", choice.isOn);
    case "audioTrack":
      return keepAudioChoice(api, fileId, choice.streamId);
    case "soundOffset":
    case "fileSoundOffset":
    case "channel":
      return undefined;
  }
}

/** Keeps a picture choice the person keeps, as the Play page does; undefined for one they do not. */
function keptPicture(api: Api, fileId: number, choice: PictureChoice): Promise<void> | undefined {
  switch (choice.kind) {
    case "subtitles":
      return keepSubtitleChoice(api, fileId, choice.key);
    case "subtitleOffset":
      return keepOffset(api, fileId, choice.ms);
    case "original":
    case "subtitleFetched":
      return undefined;
  }
}
