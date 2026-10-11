// SPDX-License-Identifier: MPL-2.0

import "./together-voice.css";

import {
  createTogetherVoice,
  partnerGainMost,
  type Api,
  type TogetherVoice,
  type VoiceSnapshot,
} from "@orpheum/sdk";
import type { TFunction } from "i18next";
import {
  createContext,
  use,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import type { AudioOutput } from "./accounts.ts";
import { ownScriptOf } from "./app-update.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { dataOrThrow } from "./problem.ts";
import { useServices, useToken } from "./services.tsx";
import { membershipHeaders, type Membership } from "./together-storage.ts";
import { monotonicMs } from "./use-together.ts";

/** The relay's ICE servers for a membership (D243 §8), as the browser's RTCPeerConnection takes them. */
async function iceServersFor(
  api: Api,
  membership: Pick<Membership, "sessionId" | "credential">,
): Promise<readonly RTCIceServer[]> {
  const answer = dataOrThrow(
    await api.GET("/together/{id}/ice", {
      params: { path: { id: membership.sessionId } },
      headers: membershipHeaders(membership.credential),
    }),
  );
  return (answer.iceServers ?? []).map((server) => ({
    urls: server.urls ?? [],
    username: server.username,
    credential: server.credential,
  }));
}

/** A person's voice, and what it knows now. */
export interface VoiceInUse {
  readonly voice: TogetherVoice;
  readonly snapshot: VoiceSnapshot;
}

/**
 * This person's voice in their session, by their membership: hearing everyone from the start
 * (D281 §1), once isReady, and left as the component goes. Made again only for another membership.
 */
export function useVoice(
  membership: Membership,
  sessionToken: string | undefined,
  isReady = true,
): VoiceInUse {
  const { api, openTogetherSocket, voiceDevices } = useServices();
  const devices = useMemo(() => voiceDevices(), [voiceDevices]);
  const { sessionId, credential } = membership;
  const voice = useMemo(
    () =>
      createTogetherVoice({
        credential,
        client: ownScriptOf(document),
        sessionToken,
        openSocket: openTogetherSocket,
        monotonicMs,
        fetchIceServers: () => iceServersFor(api, { sessionId, credential }),
        devices,
      }),
    [api, sessionId, credential, sessionToken, openTogetherSocket, devices],
  );
  useEffect(
    () => () => {
      voice.leave();
      devices.close();
    },
    [voice, devices],
  );
  useEffect(() => {
    if (isReady) {
      voice.hear();
    }
  }, [voice, isReady]);
  const snapshot = useSyncExternalStore(voice.subscribe, voice.snapshot);
  return { voice, snapshot };
}

/**
 * The host's voice in the session this tab hosts, one for the whole tab, so the status tray's
 * pop-up and the player share it (D281 §5); undefined while the tab hosts none.
 */
const HostVoiceContext = createContext<VoiceInUse | undefined>(undefined);

/** The host's voice, from HostVoiceProvider; undefined while this tab hosts no session. */
export function useHostVoice(): VoiceInUse | undefined {
  return use(HostVoiceContext);
}

/** Keeps the host's voice while this tab hosts membership, for every screen under it. */
export function HostVoiceProvider(props: {
  readonly membership: Membership | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  return props.membership === undefined ? (
    <>{props.children}</>
  ) : (
    <HostVoiceOf membership={props.membership}>{props.children}</HostVoiceOf>
  );
}

function HostVoiceOf(props: {
  readonly membership: Membership;
  readonly children: ReactNode;
}): JSX.Element {
  const voice = useVoice(props.membership, useToken());
  return <HostVoiceContext value={voice}>{props.children}</HostVoiceContext>;
}

/**
 * Where voice stands, in a line: connecting, alone, or whom it hears (watch-together.md: What each
 * person sees).
 */
function voiceStatusText(t: TFunction, snapshot: VoiceSnapshot): string {
  if (!snapshot.isConnected) {
    return t("together.voice.connecting");
  }
  if (snapshot.partners.length === 0) {
    return t("together.voice.alone");
  }
  const namesIn = (state: string): string[] =>
    snapshot.partners.filter((partner) => partner.state === state).map((partner) => partner.name);
  const connected = namesIn("connected");
  if (connected.length > 0) {
    return t("together.voice.connectedTo", { names: connected });
  }
  // A call that fails turns voice off, so every call not connected is still connecting.
  return t("together.voice.connectingTo", { names: namesIn("connecting") });
}

/** How often the Talk button's ring follows the voice going out, in ms. */
export const talkRingEveryMs = 100;

/**
 * How far the Talk button's ring reaches for a voice's level, from 0 to 1: a speaking voice
 * (voiceTargetLevel, 0.1) fills it, and a quiet one still shows, the scale a square root's.
 */
export function talkRingOf(level: number): number {
  return Math.min(1, Math.sqrt(Math.max(0, level) / 0.1));
}

/** A microphone, struck through while off. */
function MicrophoneIcon(props: { readonly isOn: boolean }): JSX.Element {
  return (
    <svg className="together-voice__icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      {props.isOn ? (
        <path d="M12 14a3 3 0 0 0 3-3V5a3 3 0 0 0-6 0v6a3 3 0 0 0 3 3zm5-3a5 5 0 0 1-10 0H5a7 7 0 0 0 6 6.9V21h2v-3.1A7 7 0 0 0 19 11z" />
      ) : (
        <path d="M19 11h-2a5 5 0 0 1-.6 2.4l1.5 1.5A7 7 0 0 0 19 11zM15 5a3 3 0 0 0-6 0v.2l6 6V5zM4.3 3 3 4.3l6 6V11a3 3 0 0 0 4.4 2.6l1.6 1.6A5 5 0 0 1 7 11H5a7 7 0 0 0 6 6.9V21h2v-3.1a7 7 0 0 0 3.4-1.3l3.3 3.3 1.3-1.3z" />
      )}
    </svg>
  );
}

/** Has the Talk button's ring follow the voice going out, every talkRingEveryMs while talking. */
function useTalkRing(
  ref: RefObject<HTMLButtonElement | null>,
  isTalking: boolean,
  voice: TogetherVoice,
): void {
  useEffect(() => {
    const button = ref.current;
    if (!isTalking || button === null) {
      return undefined;
    }
    const follow = setInterval(() => {
      button.style.setProperty("--level", String(talkRingOf(voice.readMicrophoneLevel())));
    }, talkRingEveryMs);
    return () => {
      clearInterval(follow);
      button.style.removeProperty("--level");
    };
  }, [ref, isTalking, voice]);
}

/** Opens the microphone, processed as the session says, at Talk's press. */
function startTalking(props: VoiceInUse): void {
  void props.voice.start({
    shouldCancelEcho: props.snapshot.tuning.shouldCancelMicrophoneEcho,
    shouldLevel: props.snapshot.tuning.shouldBrowserLevelMicrophone,
  });
}

/**
 * As an iPhone without headphones first talks: while its microphone is open, iOS sends the film's
 * sound to the earpiece, and only headphones carry the film and the voices together (the owner,
 * 2026-10-11; idiosyncrasies.md). The microphone is already on; OK closes the notice, and Talk
 * turns it off for whoever would rather hear the film (the owner: "just have OK").
 */
function HeadphonesWarning(props: { readonly onRead: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <ModalDialog
      className="together-voice__headphones"
      role="alertdialog"
      label={t("together.voice.earpiece")}
      onClose={props.onRead}
    >
      <p>{t("together.voice.earpiece")}</p>
      <button type="button" onClick={props.onRead}>
        {t("together.voice.ok")}
      </button>
    </ModalDialog>
  );
}

/**
 * Talk, one toggle (D281 §7), a round microphone button, the owner's choice B: struck through
 * while off; in the status blue while on, a ring around it growing with the voice going out. A
 * press opens the microphone, from the press it needs, and the next closes it, still hearing
 * everyone. Its name is for screen readers and a pointer's hover.
 */
function TalkToggle(
  props: VoiceInUse & {
    /** Whether the first press warns that only headphones carry the film and voice together. */
    readonly shouldWarnFirst: boolean;
  },
): JSX.Element {
  const { t } = useTranslation();
  const { voice, snapshot } = props;
  const isTalking = snapshot.status === "on";
  const ref = useRef<HTMLButtonElement>(null);
  // Warned once a page: the person has read it, and a warning at every press would nag.
  const [warning, setWarning] = useState<"none" | "shown" | "read">("none");
  useTalkRing(ref, isTalking, voice);
  const name = isTalking ? t("together.voice.stopTalking") : t("together.voice.talk");
  return (
    <>
      <button
        ref={ref}
        type="button"
        className="together-voice__talk"
        aria-pressed={isTalking}
        aria-label={name}
        title={name}
        data-starting={snapshot.status === "starting" ? "" : undefined}
        disabled={snapshot.status === "starting"}
        onClick={() => {
          if (isTalking) {
            voice.stop();
          } else {
            if (props.shouldWarnFirst && warning === "none") {
              setWarning("shown");
            }
            startTalking(props);
          }
        }}
      >
        <MicrophoneIcon isOn={isTalking} />
      </button>
      {warning === "shown" ? (
        <HeadphonesWarning
          onRead={() => {
            setWarning("read");
          }}
        />
      ) : null}
    </>
  );
}

/** How loud one person's voice plays, apart from the film's volume and everyone else's (D281 §4). */
export function PersonVoiceVolume(
  props: VoiceInUse & { readonly memberId: number; readonly name: string },
): JSX.Element {
  const { t } = useTranslation();
  const gain = props.snapshot.partnerGains[props.memberId] ?? 1;
  return (
    <label className="together-voice__partner">
      {t("together.voice.personVolume", { name: props.name })}{" "}
      <input
        type="range"
        min={0}
        max={partnerGainMost * 100}
        value={Math.round(gain * 100)}
        onChange={(event) => {
          props.voice.setPartnerGain(props.memberId, Number(event.target.value) / 100);
        }}
      />
    </label>
  );
}

/** Each other person's voice volume, one per person whatever their devices. */
export function VoiceVolumes(props: VoiceInUse): JSX.Element | null {
  const { t } = useTranslation();
  const people = new Map(props.snapshot.partners.map((each) => [each.memberId, each.name]));
  if (people.size === 0) {
    return null;
  }
  return (
    <fieldset className="together-voice__volumes">
      <legend>{t("together.voice.volumes")}</legend>
      {[...people].map(([memberId, name]) => (
        <PersonVoiceVolume key={memberId} {...props} memberId={memberId} name={name} />
      ))}
    </fieldset>
  );
}

/**
 * Whether this browser's sound goes to the earpiece while its microphone is open: an iPhone's or an
 * iPad's, told by a touch screen and Safari's audio session (feature, never the User-Agent). Its
 * film went silent for a guest on the phone's speaker who turned voice on, until some 20 s after he
 * turned it off (the owner's test, 2026-10-08; WebKit 230902, 236219; idiosyncrasies.md).
 */
export function movesSoundToTheEarpiece(
  browser: { readonly maxTouchPoints: number; readonly audioSession?: unknown } = navigator,
): boolean {
  return browser.maxTouchPoints > 0 && "audioSession" in browser;
}

/**
 * Voice in a Watch Together session, for the host and every viewer (watch-together.md, Voice;
 * D281): everyone is heard from the start; Talk, one toggle, opens the microphone and closes it
 * again; and each person's volume, apart from the film's. A call that fails is said,
 * with a button to connect again. Headphones are the supported way to keep the film out of the
 * microphone (D63), so the controls say so. A microphone that cannot open says why.
 */
export function VoiceControls(
  props: VoiceInUse & {
    /** How this device makes sound: an iPhone without headphones is warned before Talk. */
    readonly audioOutput: AudioOutput | undefined;
  },
): JSX.Element {
  const { t } = useTranslation();
  const { voice, snapshot } = props;
  const isTalking = snapshot.status === "on";
  return (
    <section className="together-voice" aria-label={t("together.voice.label")}>
      {snapshot.isHearing ? (
        <output className="together-voice__status">{voiceStatusText(t, snapshot)}</output>
      ) : null}
      {snapshot.failure === undefined ? null : (
        <p role="alert">{t(`together.voice.failure.${snapshot.failure}`)}</p>
      )}
      {snapshot.browserReason === undefined ? null : (
        <p className="together-voice__reason">
          {t("together.voice.browserReason", { reason: snapshot.browserReason })}
        </p>
      )}
      {snapshot.failure === "unreachable" ? (
        <button
          type="button"
          onClick={() => {
            voice.hear();
          }}
        >
          {t("together.voice.again")}
        </button>
      ) : null}
      <TalkToggle
        voice={voice}
        snapshot={snapshot}
        shouldWarnFirst={props.audioOutput !== "headphones" && movesSoundToTheEarpiece()}
      />
      {isTalking ? (
        <output className="together-voice__talking">{t("together.voice.talking")}</output>
      ) : null}
      {snapshot.status === "starting" ? <output>{t("together.voice.starting")}</output> : null}
      <VoiceVolumes voice={voice} snapshot={snapshot} />
      <p className="together-voice__hint">{t("together.voice.headphones")}</p>
    </section>
  );
}
