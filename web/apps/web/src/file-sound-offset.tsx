// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import type { TFunction } from "i18next";
import { reportFailure, type Api } from "@orpheum/sdk";
import { use, useEffect, useState, useSyncExternalStore, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useIsGuest } from "./guest-page.ts";
import { latestWrite } from "./latest-write.ts";
import { ListEntry, ListOption } from "./list-box.tsx";
import { throwIfRefused } from "./problem.ts";
import type { SuggestedSoundOffset } from "./problem-report.tsx";
import { useServices } from "./services.tsx";
import {
  heldToLongest,
  isEarlierRefusedFor,
  setFileSoundOffsetInThisTab,
  soundOffsetKeptAfterMs,
  SoundOffsetInput,
  tabValue,
  TimingNotes,
  useDeviceSoundOffsetMs,
  useFileSoundOffset,
  type FileOffsetNow,
  type FileOffsetOf,
} from "./sound-delay.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

/** What failed last for a file's own timing: keeping the person's (fileNotKept), or making it everyone's. */
interface FileOffsetFailure {
  readonly fileId: number;
  readonly key: "fileNotKept" | "everyonesNotKept";
  readonly cause: unknown;
}

/**
 * The failure this tab last met for a file's timing, held past the menu's closing: Make it
 * everyone's closes the menu as any choice does, so its failure shows as the menu opens again.
 */
const failureInThisTab = tabValue<FileOffsetFailure | undefined>(undefined);

/** Forgets the failure this tab met for a file's timing, for a test. */
export function forgetFileSoundOffsetFailure(): void {
  failureInThisTab.set(undefined);
}

/** Keeps the person's own offset for a file (D537 §2), which overrides every person's for them. */
async function keepOwnFileOffset(api: Api, fileId: number, soundOffsetMs: number): Promise<void> {
  throwIfRefused(
    await api.PUT("/files/{id}/preferences", {
      params: { path: { id: fileId } },
      body: { soundOffsetMs },
    }),
  );
}

/**
 * Sets a file's sound offset for every person, an admin's (D537 §2): the one way to, for the Sound
 * menu's Make it everyone's and a report's Apply to everyone alike (system-problem-reports.tsx).
 */
export async function setEveryonesSoundOffset(
  api: Api,
  fileId: number,
  soundOffsetMs: number,
): Promise<void> {
  throwIfRefused(
    await api.PUT("/files/{id}/sound-offset", {
      params: { path: { id: fileId } },
      body: { soundOffsetMs },
    }),
  );
}

/**
 * Makes ms the file's offset for every person, an admin's (D537 §3), then forgets the admin's own,
 * which now says the same, so a later change for everyone reaches them too.
 */
async function makeEveryones(api: Api, fileId: number, soundOffsetMs: number): Promise<void> {
  await setEveryonesSoundOffset(api, fileId, soundOffsetMs);
  throwIfRefused(
    await api.PUT("/files/{id}/preferences", {
      params: { path: { id: fileId } },
      body: { isSoundOffsetEveryones: true },
    }),
  );
}

/**
 * The Sound menu's "This file" (D537, D538 §8): the file's own timing for the person, a stepper as
 * "This device" is, starting at their own for the file, else every person's; a step keeps their own,
 * the last of a run as presses settle, as the device's is. The sound plays with both, added, and the
 * two are shown apart. Beneath it, where their own differs from every person's, one row: Make it
 * everyone's for an admin, which sets every person's to it (PUT /files/{id}/sound-offset), and Tell
 * the admin for anyone else, a problem report suggesting it (D537 §5, onTellTheAdmin). Its notes say
 * what failed and, as the device's do, that the film cannot play its sound earlier. A guest keeps no
 * preference (D469 §4), so it shows none.
 */
export function FileSoundOffsetEntries(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly file: FileOffsetOf;
  /** The device's offset the playback started with, which plays with this one. */
  readonly deviceInitialMs: number;
  readonly onTellTheAdmin: (suggested: SuggestedSoundOffset) => void;
}): JSX.Element | null {
  const isGuest = useIsGuest();
  return isGuest ? null : <FileTiming {...props} />;
}

/** This tab's last failure to keep a file's own offset, where it is this file's. */
function useFailureOf(fileId: number): ReturnType<typeof failureInThisTab.get> | undefined {
  const failure = useSyncExternalStore(
    failureInThisTab.hear,
    failureInThisTab.get,
    failureInThisTab.get,
  );
  return failure?.fileId === fileId ? failure : undefined;
}

function FileTiming(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly file: FileOffsetOf;
  readonly deviceInitialMs: number;
  readonly onTellTheAdmin: (suggested: SuggestedSoundOffset) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const isAdmin = use(IsAdminContext);
  const now = useFileSoundOffset(props.file);
  const deviceMs = useDeviceSoundOffsetMs(props.deviceInitialMs);
  const shownFailure = useFailureOf(props.file.fileId);
  const [isEarlierRefused, setIsEarlierRefused] = useState(false);
  const writes = useOwnKept(api);
  const { fileId } = props.file;
  const setTo = (wanted: number): void => {
    const next = heldToLongest(wanted);
    setFileSoundOffsetInThisTab({ fileId, everyonesMs: now.everyonesMs, ownMs: next });
    setIsEarlierRefused(isEarlierRefusedFor(props.videoRef, deviceMs + next));
    writes.write({ fileId, ms: next });
  };
  const own = now.ownMs;
  const isOwnApart = own !== undefined && own !== now.everyonesMs;
  return (
    <>
      <ListEntry isKeptOpen isInsetAsChoices>
        <SoundOffsetInput
          label={t("play.soundOffset.thisFile")}
          fieldLabel={t("play.soundOffset.thisFileField")}
          valueText={fileTimingText(t, now.ms)}
          ms={now.ms}
          stepLabels={{
            earlier: t("play.soundOffset.thisFileEarlier"),
            later: t("play.soundOffset.thisFileLater"),
          }}
          onChange={setTo}
        />
        <TimingNotes
          isEarlierRefused={isEarlierRefused}
          failure={shownFailure}
          notKeptKey={`play.soundOffset.${shownFailure?.key ?? "fileNotKept"}`}
        />
      </ListEntry>
      {isOwnApart ? (
        <FileOffsetAction
          isAdmin={isAdmin}
          onEveryones={() => {
            void madeEveryones(api, writes.settled, { fileId, ownMs: own });
          }}
          onTellTheAdmin={() => {
            writes.flush();
            props.onTellTheAdmin({ fileId, soundOffsetMs: own });
          }}
        />
      ) : null}
    </>
  );
}

/** The one row under "This file": Make it everyone's for an admin, Tell the admin for anyone else. */
function FileOffsetAction(props: {
  readonly isAdmin: boolean;
  readonly onEveryones: () => void;
  readonly onTellTheAdmin: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return props.isAdmin ? (
    <ListOption onChoose={props.onEveryones}>{t("play.soundOffset.makeEveryones")}</ListOption>
  ) : (
    <ListOption onChoose={props.onTellTheAdmin}>{t("play.soundOffset.tellTheAdmin")}</ListOption>
  );
}

/**
 * Makes the admin's own offset for a file every person's once their own writes have settled, so
 * none lands after it; then the file plays with every person's, now theirs. A failure is held for
 * the menu to show as it opens again.
 */
async function madeEveryones(
  api: Api,
  settled: () => Promise<void>,
  own: { readonly fileId: number; readonly ownMs: number },
): Promise<void> {
  const { fileId, ownMs } = own;
  await settled();
  try {
    await makeEveryones(api, fileId, ownMs);
    failureInThisTab.set(undefined);
    const now: FileOffsetNow = { fileId, everyonesMs: ownMs, ownMs: undefined };
    setFileSoundOffsetInThisTab(now);
  } catch (cause: unknown) {
    reportFailure(cause);
    failureInThisTab.set({ fileId, key: "everyonesNotKept", cause });
  }
}

/**
 * Keeps the person's own offset for a file as presses settle, one write at a time, as the device's
 * is (sound-delay.tsx's useKeptOnDevice); a press still waiting is kept as the menu closes.
 */
function useOwnKept(api: Api): {
  readonly write: (own: { readonly fileId: number; readonly ms: number }) => void;
  readonly flush: () => void;
  readonly settled: () => Promise<void>;
} {
  const [writes] = useState(() =>
    latestWrite<{ readonly fileId: number; readonly ms: number }>(
      async ({ fileId, ms }) => {
        try {
          await keepOwnFileOffset(api, fileId, ms);
        } catch (cause: unknown) {
          failureInThisTab.set({ fileId, key: "fileNotKept", cause });
          throw cause;
        }
      },
      soundOffsetKeptAfterMs,
      {
        kept: () => {
          failureInThisTab.set(undefined);
        },
        failed: reportFailure,
      },
    ),
  );
  useEffect(
    () => () => {
      writes.flush();
    },
    [writes],
  );
  return writes;
}

/** What "This file" says while not 0: how much later or earlier the file's sound plays. */
function fileTimingText(t: TFunction, ms: number): string {
  return ms > 0
    ? t("play.soundOffset.fileLater", { ms })
    : t("play.soundOffset.fileEarlier", { ms: -ms });
}
