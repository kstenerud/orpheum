// SPDX-License-Identifier: MPL-2.0

import type { TFunction } from "i18next";
import {
  canMoveSoundEarlier,
  longestSoundOffsetMs,
  newAudioContext,
  soundOffset,
} from "@orpheum/player";
import { reportFailure } from "@orpheum/sdk";
import {
  useEffect,
  useState,
  useSyncExternalStore,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { listMyDevices, setMyDeviceSoundOffset } from "./accounts.ts";
import { latestWrite } from "./latest-write.ts";
import { Cluster, Stack } from "./layout.tsx";
import { problemText } from "./problem.ts";
import { soundLatencyMemory } from "./sound-latency.ts";
import { useSoundPart } from "./sound-panel.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** How much each press moves the sound, in milliseconds. */
export const soundOffsetStepMs = 50;

/**
 * How long after the last press the device keeps the timing: a run of presses is one write, of
 * where it ended. A justified constant: past a quick run of presses, short beside a person's wait
 * to hear the change.
 */
export const soundOffsetKeptAfterMs = 500;

/**
 * The offset this tab last set, which a playback asked for before it, as a film page's warm-up is,
 * did not hear of: the owner set 0 during one film and the next started at the 50 before it
 * (2026-10-24). Undefined until the tab sets one.
 */
const setInThisTab: { ms: number | undefined } = { ms: undefined };

/**
 * What hears the offset this tab sets: every control drawn shows and applies the one value, the
 * hidden one under the film and the player's ⁝ alike (D407 §8), so neither moves from a stale one.
 */
const tabListeners = new Set<() => void>();

/** Sets the offset this tab last set, telling every control. */
function setTabMs(ms: number | undefined): void {
  setInThisTab.ms = ms;
  for (const listener of tabListeners) {
    listener();
  }
}

function hearTab(listener: () => void): () => void {
  tabListeners.add(listener);
  return () => {
    tabListeners.delete(listener);
  };
}

function tabMs(): number | undefined {
  return setInThisTab.ms;
}

/** Forgets the offset this tab set, for a test. */
export function forgetSoundOffsetSetInThisTab(): void {
  setTabMs(undefined);
}

/**
 * The timing of this device's sound against its picture, under the video, which a person sets once
 * on a device that plays one of them late and the browser cannot tell: the owner's TCL TV, in TV
 * Bro, plays its sound half a second late (2026-10-24). Later holds the sound back; earlier moves a
 * streamed film's sound chunks forward, which a film playing its own file cannot, and says so. It is
 * the device's own, kept by the core with the device and given with each playback (initialMs), or
 * as this tab last set it, every control drawn showing the one value; it applies at once, and the
 * player counts Web Audio's own latency. A Watch Together viewer with no device of its own still
 * sets it, kept by the tab alone (the owner, 2026-11-06: "Put a control in watch together"). As an entry of ⁝'s list
 * (isInAList, player-menus.tsx), its text is on a line of its own, its buttons in a row below.
 */
export function SoundOffsetControl(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly initialMs: number;
  readonly isInAList?: boolean;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { session } = useServices();
  const offsetMs = useSyncExternalStore(hearTab, tabMs, tabMs) ?? props.initialMs;
  const { keep, failure } = useKeptOnDevice();
  // Whether the sound is wanted earlier than this film can play it: it plays as its own file.
  const [isEarlierRefused, setIsEarlierRefused] = useState(false);
  useEffect(() => {
    const video = props.videoRef.current;
    if (video !== null) {
      soundOffset(video, newAudioContext, soundLatencyMemory()).setMs(offsetMs);
    }
  }, [offsetMs, props.videoRef]);
  const deviceId = session.deviceId();
  const setTo = (wanted: number): void => {
    const next = Math.min(Math.max(wanted, -longestSoundOffsetMs), longestSoundOffsetMs);
    setTabMs(next);
    const video = props.videoRef.current;
    setIsEarlierRefused(next < 0 && video !== null && !canMoveSoundEarlier(video));
    if (deviceId !== undefined) {
      keep({ deviceId, ms: next });
    }
  };
  useMovedByTheRemote(props.isInAList === true ? undefined : offsetMs, setTo);
  const buttons = (
    <>
      <button
        type="button"
        onClick={() => {
          setTo(offsetMs - soundOffsetStepMs);
        }}
      >
        {t("play.soundOffset.earlier", { ms: soundOffsetStepMs })}
      </button>{" "}
      <button
        type="button"
        onClick={() => {
          setTo(offsetMs + soundOffsetStepMs);
        }}
      >
        {t("play.soundOffset.later", { ms: soundOffsetStepMs })}
      </button>
    </>
  );
  return (
    <TimingRow
      isInAList={props.isInAList === true}
      text={timingText(t, offsetMs)}
      notes={<TimingNotes isEarlierRefused={isEarlierRefused} failure={failure} />}
    >
      {buttons}
    </TimingRow>
  );
}

/**
 * The sound timing starting from the offset kept with the device this tab is signed in on, and from
 * in step for a tab on none, where no playback gives one: a Watch Together viewer's under the film
 * (the owner, 2026-11-06: "Make it respect the sound offset. Put a control in watch together"), as
 * the server gives a guest's playback none, and the device lab's sound timing test's (lab-sound.tsx).
 */
export function DeviceSoundOffsetControl(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element | null {
  return <SoundOffsetControl videoRef={props.videoRef} initialMs={useThisDeviceSoundOffsetMs()} />;
}

/** The sound offset kept with the device this tab is signed in on; 0 for none, or until read. */
function useThisDeviceSoundOffsetMs(): number {
  const { api, session } = useServices();
  const deviceId = session.deviceId();
  const [devices] = useResource(`my-devices-sound/${String(deviceId)}`, async () =>
    deviceId === undefined ? [] : await listMyDevices(api),
  );
  return devices.state === "loaded"
    ? (devices.value.find((each) => each.id === deviceId)?.soundOffsetMs ?? 0)
    : 0;
}

/**
 * Has a cast's remote move the timing too, as the control's own press (sound-panel.ts): the control
 * under the video says how it stands, ms, and ⁝'s copy, which comes and goes, says nothing
 * (undefined).
 */
function useMovedByTheRemote(ms: number | undefined, setTo: (ms: number) => void): void {
  useSoundPart(ms === undefined ? undefined : { kind: "soundOffset", ms }, (choice) => {
    if (choice.kind === "soundOffset") {
      setTo(choice.ms);
    }
  });
}

/**
 * The timing's parts under the video, or as an entry of ⁝'s list (the cast remote's too, D444): there its text has a line of its
 * own above the buttons, as its length changes with each press, and in one row with them it moved
 * Earlier between the row and the next on the owner's TCL TV (2026-11-06).
 */
export function TimingRow(props: {
  readonly isInAList: boolean;
  readonly text: string;
  readonly notes: ReactNode;
  readonly children: ReactNode;
}): JSX.Element {
  return props.isInAList ? (
    <Stack space="1">
      <span>{props.text}</span>
      <Cluster space="2">{props.children}</Cluster>
      {props.notes}
    </Stack>
  ) : (
    <div className="play__sound-delay">
      <span>{props.text}</span> {props.children}
      {props.notes}
    </div>
  );
}

/**
 * Keeps the timing with the device as presses settle (the audit of 2026-10-25, W9): the last of a
 * run, soundOffsetKeptAfterMs after it, one write at a time, so a slower earlier write over the
 * relay cannot land after a later one and leave the core an older value than the page shows. A
 * press still waiting is kept as the control goes. A write that failed is reported, and its cause
 * held until one is kept, for the control to say so.
 */
function useKeptOnDevice(): {
  readonly keep: (offset: { readonly deviceId: number; readonly ms: number }) => void;
  readonly failure: { readonly cause: unknown } | undefined;
} {
  const { api } = useServices();
  const [failure, setFailure] = useState<{ readonly cause: unknown } | undefined>(undefined);
  const [writes] = useState(() =>
    latestWrite<{ readonly deviceId: number; readonly ms: number }>(
      async ({ deviceId, ms }) => {
        await setMyDeviceSoundOffset(api, deviceId, ms);
      },
      soundOffsetKeptAfterMs,
      {
        kept: () => {
          setFailure(undefined);
        },
        failed: (cause) => {
          reportFailure(cause);
          setFailure({ cause });
        },
      },
    ),
  );
  useEffect(
    () => () => {
      writes.flush();
    },
    [writes],
  );
  return { keep: writes.write, failure };
}

/** What the timing says beneath it: that the film cannot play its sound earlier, or a timing not kept. */
function TimingNotes(props: {
  readonly isEarlierRefused: boolean;
  readonly failure: { readonly cause: unknown } | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { isEarlierRefused, failure } = props;
  return (
    <>
      {isEarlierRefused ? (
        <p className="play__reason">{t("play.soundOffset.cannotEarlier")}</p>
      ) : null}
      {failure === undefined ? null : (
        <p className="play__reason">
          {t("thisDevice.soundOffsetNotKept", { problem: problemText(t, failure.cause) })}
        </p>
      )}
    </>
  );
}

/** What the timing says: in step, or how much later or earlier the sound plays. */
function timingText(t: TFunction, offsetMs: number): string {
  if (offsetMs === 0) {
    return t("play.soundOffset.inStep");
  }
  return offsetMs > 0
    ? t("play.soundOffset.laterBy", { ms: offsetMs })
    : t("play.soundOffset.earlierBy", { ms: -offsetMs });
}
