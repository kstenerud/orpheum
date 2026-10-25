// SPDX-License-Identifier: MPL-2.0

import { failureText, openLoopback, type Loopback } from "@orpheum/sdk";
import { useEffect, useReducer, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { LabFrame, LabSendButton, yesNo } from "./lab-frame.tsx";
import { useLabSending } from "./lab-send.ts";
import { elapsedText, GapFinder } from "./lab-timing.ts";
import { useServices } from "./services.tsx";

/** How often the run's log goes to the server, so it survives the page dying: the review's §2 probe. */
const heartbeatEveryMs = 30_000;

/** How many events the run keeps and sends, the latest: about 20 KiB, inside the endpoint's 64 KiB. */
const eventsKept = 400;

/** A level above this is sound flowing; below it, silence. */
const flowingAbove = 0.001;

/** One thing that happened in a run, at its seconds since the start. */
interface RunEvent {
  readonly atS: number;
  readonly what: string;
  readonly detail?: string;
}

/** Whether this page runs as an installed home-screen app rather than in a browser's tab. */
function isStandalone(): boolean {
  const isStandaloneByMedia =
    typeof globalThis.matchMedia === "function" &&
    globalThis.matchMedia("(display-mode: standalone)").matches;
  return isStandaloneByMedia || Reflect.get(navigator, "standalone") === true;
}

/** How loud the call plays back the microphone: quietly, against feedback through a speaker. */
const playBackVolume = 0.2;

/**
 * One endurance run of a phone acting as the remote (casting-review-2026-10-25.md §2): a screen
 * wake lock held and asked again each time the page is shown, the microphone through a WebRTC call
 * with the page itself played out by an <audio> element (not Web Audio), the received level
 * sampled, a 1 s ticker whose gaps say the page was frozen, and the page's visibility.
 */
class RemoteRun {
  readonly startedAtMs = Date.now();
  /** How long the run has gone, as of its last tick. */
  elapsedMs = 0;
  readonly events: RunEvent[] = [];
  eventsDropped = 0;
  wakeLocksAcquired = 0;
  wakeLocksReleased = 0;
  levelSamples = 0;
  flowingSamples = 0;
  lastLevel: number | undefined;
  isFlowing: boolean | undefined;
  private readonly gaps = new GapFinder(this.startedAtMs);
  private wakeLock: WakeLockSentinel | undefined;
  private microphone: MediaStream | undefined;
  private loopback: Loopback | undefined;
  private readonly timers: ReturnType<typeof setInterval>[] = [];
  private readonly stopListening: (() => void)[] = [];
  /** Plays the call back: an element, never Web Audio, which a locked iPhone suspends (review §2). */
  private readonly audio = new Audio();

  constructor(
    private readonly onChange: () => void,
    private readonly onHeartbeat: () => void,
  ) {}

  /** Notes what happened now. */
  note(what: string, detail?: string): void {
    const atS = Math.round((Date.now() - this.startedAtMs) / 100) / 10;
    this.events.push(detail === undefined ? { atS, what } : { atS, what, detail });
    if (this.events.length > eventsKept) {
      this.events.shift();
      this.eventsDropped += 1;
    }
    this.onChange();
  }

  /** Starts every part, from the press that started the run. */
  async start(): Promise<void> {
    this.note("started", isStandalone() ? "standalone" : "browser tab");
    this.listen(document, "visibilitychange", () => {
      this.note(document.visibilityState);
      if (document.visibilityState === "visible") {
        void this.holdWakeLock();
      }
    });
    for (const lifecycle of ["pagehide", "pageshow", "freeze", "resume", "online", "offline"]) {
      this.listen(
        lifecycle === "freeze" || lifecycle === "resume" ? document : globalThis,
        lifecycle,
        () => {
          this.note(lifecycle);
        },
      );
    }
    this.timers.push(
      setInterval(() => {
        this.tick();
      }, 1000),
      setInterval(() => {
        this.onHeartbeat();
      }, heartbeatEveryMs),
    );
    await this.holdWakeLock();
    await this.openMicrophone();
  }

  /** Ends the run. */
  stop(): void {
    for (const timer of this.timers) {
      clearInterval(timer);
    }
    for (const stop of this.stopListening) {
      stop();
    }
    void this.wakeLock?.release().catch(() => undefined);
    this.loopback?.close();
    this.audio.pause();
    this.audio.srcObject = null;
    for (const track of this.microphone?.getTracks() ?? []) {
      track.stop();
    }
    this.note("stopped");
  }

  /** What the run found, as JSON the lab endpoint takes. */
  result(): Record<string, unknown> {
    return {
      elapsedMs: Date.now() - this.startedAtMs,
      isStandalone: isStandalone(),
      userAgent: navigator.userAgent,
      gaps: this.gaps.gaps,
      longestGapMs: this.gaps.longestMs(),
      wakeLocksAcquired: this.wakeLocksAcquired,
      wakeLocksReleased: this.wakeLocksReleased,
      levelSamples: this.levelSamples,
      flowingSamples: this.flowingSamples,
      isFlowing: this.isFlowing,
      eventsDropped: this.eventsDropped,
      events: this.events,
    };
  }

  /** The gaps the ticker found. */
  gapCount(): number {
    return this.gaps.gaps.length;
  }

  longestGapMs(): number {
    return this.gaps.longestMs();
  }

  private listen(target: EventTarget, type: string, listener: () => void): void {
    target.addEventListener(type, listener);
    this.stopListening.push(() => {
      target.removeEventListener(type, listener);
    });
  }

  private tick(): void {
    const now = Date.now();
    this.elapsedMs = now - this.startedAtMs;
    const gap = this.gaps.tick(now);
    if (gap !== undefined) {
      this.note("frozen", `${String(gap.lastedMs)} ms`);
    }
    void this.sampleLevel();
    this.onChange();
  }

  private async sampleLevel(): Promise<void> {
    if (this.loopback === undefined) {
      return;
    }
    const level = await this.loopback.level().catch(() => undefined);
    this.levelSamples += 1;
    this.lastLevel = level;
    const isFlowing = level !== undefined && level > flowingAbove;
    if (isFlowing) {
      this.flowingSamples += 1;
    }
    if (isFlowing !== this.isFlowing) {
      this.isFlowing = isFlowing;
      this.note(isFlowing ? "soundFlowing" : "soundSilent");
    }
  }

  private async holdWakeLock(): Promise<void> {
    if (!("wakeLock" in navigator)) {
      this.note("wakeLockMissing");
      return;
    }
    try {
      const lock = await navigator.wakeLock.request("screen");
      this.wakeLock = lock;
      this.wakeLocksAcquired += 1;
      this.note("wakeLockAcquired");
      lock.addEventListener("release", () => {
        this.wakeLocksReleased += 1;
        this.note("wakeLockReleased");
      });
    } catch (cause) {
      this.note("wakeLockRefused", failureText(cause));
    }
  }

  private async openMicrophone(): Promise<void> {
    try {
      const microphone = await navigator.mediaDevices.getUserMedia({ audio: true });
      this.microphone = microphone;
      const [track] = microphone.getAudioTracks();
      if (track === undefined) {
        this.note("microphoneHasNoTrack");
        return;
      }
      for (const change of ["mute", "unmute", "ended"]) {
        this.listen(track, change, () => {
          this.note(`microphone ${change}`);
        });
      }
      this.loopback = await openLoopback(track);
      this.note("callOpen");
      this.audio.srcObject = this.loopback.remote;
      this.audio.volume = playBackVolume;
      await this.audio.play();
    } catch (cause) {
      this.note("microphoneOrCallFailed", failureText(cause));
    }
  }
}

/** The run's summary, large, for a glance at a phone left on a table. */
function RunSummary(props: { readonly run: RemoteRun }): JSX.Element {
  const { t } = useTranslation();
  const { run } = props;
  const flowingShare =
    run.levelSamples === 0 ? 0 : Math.round((100 * run.flowingSamples) / run.levelSamples);
  return (
    <dl className="lab__facts lab__huge">
      <dt>{t("lab.remote.elapsed")}</dt>
      <dd>{elapsedText(run.elapsedMs)}</dd>
      <dt>{t("lab.remote.sound")}</dt>
      <dd>
        {t("lab.remote.soundIs", { flowing: yesNo(t, run.isFlowing), share: String(flowingShare) })}
      </dd>
      <dt>{t("lab.remote.frozen")}</dt>
      <dd>
        {t("lab.remote.frozenIs", {
          count: String(run.gapCount()),
          longest: String(Math.round(run.longestGapMs() / 1000)),
        })}
      </dd>
      <dt>{t("lab.remote.wakeLock")}</dt>
      <dd>
        {t("lab.remote.wakeLockIs", {
          acquired: String(run.wakeLocksAcquired),
          released: String(run.wakeLocksReleased),
        })}
      </dd>
    </dl>
  );
}

/** The run's last events, newest first. */
function RunEvents(props: { readonly run: RemoteRun }): JSX.Element {
  const { t } = useTranslation();
  return (
    <ol className="lab__log" reversed>
      {props.run.events
        .slice(-12)
        .toReversed()
        .map((event) => (
          <li key={`${String(event.atS)}-${event.what}`}>
            {t("lab.remote.event", {
              at: String(event.atS),
              what: event.what,
              detail: event.detail ?? "",
            })}
          </li>
        ))}
    </ol>
  );
}

/**
 * The phone remote's endurance probe (plans/tv-device-tests.md; casting-review-2026-10-25.md §2):
 * run for the length of a film with the screen locked and apps switched, once in a Safari tab and
 * once as a home-screen app; its log reaches the server every 30 s, so it survives the page dying.
 */
export function LabRemote(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [run, setRun] = useState<RemoteRun | undefined>();
  const [, redraw] = useReducer((count: number) => count + 1, 0);
  const { sending, send } = useLabSending(api, "remote");
  useEffect(() => () => run?.stop(), [run]);
  const start = (): void => {
    const started: RemoteRun = new RemoteRun(redraw, () => {
      send(started.result());
    });
    setRun(started);
    void started.start();
  };
  const stop = (): void => {
    if (run !== undefined) {
      run.stop();
      send(run.result());
      setRun(undefined);
    }
  };
  return (
    <LabFrame id="lab-remote" heading={t("lab.remote.heading")}>
      <p className="lab__lead">{t("lab.remote.explain")}</p>
      <p className="lab__lead">{t("lab.remote.twice")}</p>
      <div className="lab__actions">
        <button type="button" className="lab__button" onClick={run === undefined ? start : stop}>
          {run === undefined ? t("lab.remote.start") : t("lab.remote.stop")}
        </button>
        <LabSendButton
          sending={sending}
          onSend={() => {
            if (run !== undefined) {
              send(run.result());
            }
          }}
        />
      </div>
      {run === undefined ? null : <RunSummary run={run} />}
      {run === undefined ? null : <RunEvents run={run} />}
    </LabFrame>
  );
}
