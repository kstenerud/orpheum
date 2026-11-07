// SPDX-License-Identifier: MPL-2.0

import "./together-voice.css";

import {
  createTogetherVoice,
  partnerGainMost,
  type BrowserVoiceDevices,
  type Api,
  type TogetherEndReason,
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

import { ownScriptOf } from "./app-update.ts";
import { useHasMicrophone } from "./has-microphone.ts";
import { Icon } from "./icon.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { volumeAfter } from "./volume-step.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices, useToken } from "./services.tsx";
import { OnlyOnThePanelsPage } from "./together-panel-place.tsx";
import { useHostsSessionOver } from "./together-session-end.ts";
import { membershipHeaders, type Membership } from "./together-storage.ts";
import { monotonicMs } from "./use-together.ts";
import { VolumeRange } from "./volume-range.tsx";

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
  /**
   * Whether the others' voices wait for a tap, the browser having refused to play them, and the
   * tap's play (the owner's test, 2026-10-11, where an iPhone heard no one until it tapped).
   */
  readonly tap: { readonly isWaiting: boolean; readonly play: () => void };
}

/**
 * This person's voice in their session, by their membership: hearing everyone from the start
 * (D281 §1), once isReady, and left as the component goes. Made again only for another membership.
 * Its devices are its own, closed with it, unless the page gives those it keeps: a guest's page has
 * them before its Join, whose tap readies them (useViewerVoiceDevices).
 */
export function useVoice(
  membership: Membership,
  sessionToken: string | undefined,
  page: { readonly isReady?: boolean; readonly devices?: BrowserVoiceDevices } = {},
): VoiceInUse {
  const isReady = page.isReady ?? true;
  const pageDevices = page.devices;
  const { api, openTogetherSocket, voiceDevices } = useServices();
  const devices = useMemo(() => pageDevices ?? voiceDevices(), [pageDevices, voiceDevices]);
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
        page: document,
        // Left, its socket closes at once; back from the back/forward cache, it connects again.
        leftPage: window,
      }),
    [api, sessionId, credential, sessionToken, openTogetherSocket, devices],
  );
  useEffect(
    () => () => {
      voice.leave();
      if (pageDevices === undefined) {
        devices.close();
      }
    },
    [voice, devices, pageDevices],
  );
  useEffect(() => {
    if (isReady) {
      voice.hear();
    }
  }, [voice, isReady]);
  const snapshot = useSyncExternalStore(voice.subscribe, voice.snapshot);
  const isWaiting = useSyncExternalStore(
    devices.subscribeToTapWaiting,
    devices.isWaitingForATap,
    devices.isWaitingForATap,
  );
  return { voice, snapshot, tap: { isWaiting, play: devices.playAtTap } };
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

/**
 * This page's voice in the Watch Together session it is in, the host's (HostVoiceProvider) or a
 * guest's (together-viewer.tsx): what the player's overlay reaches the people's volumes and Talk
 * from (D440 §2, §3); undefined outside a session.
 */
export const SessionVoiceContext = createContext<VoiceInUse | undefined>(undefined);

/** This page's voice in its session (SessionVoiceContext); undefined outside one. */
export function useSessionVoice(): VoiceInUse | undefined {
  return use(SessionVoiceContext);
}

/**
 * Keeps the host's voice while this tab hosts membership, for every screen under it. The screens sit
 * under the same elements with a session or none, so starting one remounts nothing: a film playing
 * as its host started a session from its overlay played again from its start, the dialog closed
 * (the owner, staging, 2026-11-07). The voice is held beside them (HostVoiceOf) and handed in.
 */
export function HostVoiceProvider(props: {
  readonly membership: Membership | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const [held, setHeld] = useState<VoiceInUse | undefined>(undefined);
  const { membership } = props;
  const voice = membership === undefined ? undefined : held;
  return (
    <>
      {membership === undefined ? null : (
        <HostVoiceOf key={membership.sessionId} membership={membership} onVoice={setHeld} />
      )}
      <HostVoiceContext value={voice}>
        <SessionVoiceContext value={voice}>{props.children}</SessionVoiceContext>
      </HostVoiceContext>
    </>
  );
}

/** The host's voice in membership's session, told onVoice as it changes and undefined as it goes. */
function HostVoiceOf(props: {
  readonly membership: Membership;
  readonly onVoice: (voice: VoiceInUse | undefined) => void;
}): null {
  const inUse = useVoice(props.membership, useToken());
  useHostLeavesAsTheSessionEnds(props.membership.sessionId, inUse.snapshot.endReason);
  const { voice, snapshot, tap } = inUse;
  const { isWaiting, play } = tap;
  const told = useMemo(
    () => ({ voice, snapshot, tap: { isWaiting, play } }),
    [voice, snapshot, isWaiting, play],
  );
  const { onVoice } = props;
  useEffect(() => {
    onVoice(told);
  }, [onVoice, told]);
  useEffect(
    () => () => {
      onVoice(undefined);
    },
    [onVoice],
  );
  return null;
}

/**
 * Takes the host's page home as the session it hosts ends, whatever page it shows (D389 §1): the
 * tab's voice is its one connection to the session on every page, a film's Play page or none, so
 * its end, as that connection is told, is heard here.
 */
function useHostLeavesAsTheSessionEnds(
  sessionId: number,
  endReason: TogetherEndReason | undefined,
): void {
  const over = useHostsSessionOver();
  useEffect(() => {
    if (endReason !== undefined) {
      over(sessionId, endReason);
    }
  }, [endReason, over, sessionId]);
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
  // A call that fails is made again (D281 §6), so every call not connected is connecting.
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
export function MicrophoneIcon(props: { readonly isOn: boolean }): JSX.Element {
  return (
    <Icon className="together-voice__icon">
      {props.isOn ? (
        <path d="M12 14a3 3 0 0 0 3-3V5a3 3 0 0 0-6 0v6a3 3 0 0 0 3 3zm5-3a5 5 0 0 1-10 0H5a7 7 0 0 0 6 6.9V21h2v-3.1A7 7 0 0 0 19 11z" />
      ) : (
        <path d="M19 11h-2a5 5 0 0 1-.6 2.4l1.5 1.5A7 7 0 0 0 19 11zM15 5a3 3 0 0 0-6 0v.2l6 6V5zM4.3 3 3 4.3l6 6V11a3 3 0 0 0 4.4 2.6l1.6 1.6A5 5 0 0 1 7 11H5a7 7 0 0 0 6 6.9V21h2v-3.1a7 7 0 0 0 3.4-1.3l3.3 3.3 1.3-1.3z" />
      )}
    </Icon>
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

/**
 * Whether voice is connecting: its own connection, or a call, made for the first time or again
 * after a failure (D281 §6); the status line moves meanwhile.
 */
function isConnecting(snapshot: VoiceSnapshot): boolean {
  return (
    !snapshot.isConnected || snapshot.partners.some((partner) => partner.state !== "connected")
  );
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
      mark="warning"
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
    /** Whether it is drawn over the film, among the overlay's buttons (D440 §3). */
    readonly isOverVideo?: boolean | undefined;
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
        className={
          props.isOverVideo === true
            ? "together-voice__talk together-voice__talk--over-video"
            : "together-voice__talk"
        }
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

/** A person in the session whose voice this page hears, by their member, whatever their devices. */
export interface VoicePerson {
  readonly memberId: number;
  readonly name: string;
}

/** Each other person in the session, once whatever their devices, in the roster's order. */
export function peopleOf(snapshot: Pick<VoiceSnapshot, "partners">): readonly VoicePerson[] {
  const people = new Map(snapshot.partners.map((each) => [each.memberId, each.name]));
  return [...people].map(([memberId, name]) => ({ memberId, name }));
}

/** How loud memberId's voice plays now, 1 as it came for one not set (D281 §4). */
export function partnerGainOf(
  snapshot: Pick<VoiceSnapshot, "partnerGains">,
  memberId: number,
): number {
  return snapshot.partnerGains[memberId] ?? 1;
}

/**
 * A person's voice's gain a step towards direction, as a remote's Up and Down move it in the
 * overlay's row (D440 §2): the film's volume step (volumeAfter), over the voice's span to
 * partnerGainMost.
 */
export function partnerGainAfter(gain: number, direction: -1 | 1): number {
  return volumeAfter(gain / partnerGainMost, direction) * partnerGainMost;
}

/**
 * The slider of one person's voice, from silent to partnerGainMost (D281 §4): the one way it is set,
 * beside its name in the Watch Together controls (PersonVoiceVolume) and upright in the overlay's
 * row (volume-row.tsx).
 */
export function PersonVoiceRange(
  props: VoiceInUse & {
    readonly memberId: number;
    readonly label?: string | undefined;
    readonly isUpright?: boolean | undefined;
    readonly isFocusable?: boolean | undefined;
  },
): JSX.Element {
  const { voice, memberId } = props;
  return (
    <VolumeRange
      value={partnerGainOf(props.snapshot, memberId)}
      most={partnerGainMost}
      {...(props.label === undefined ? {} : { label: props.label })}
      isUpright={props.isUpright}
      isFocusable={props.isFocusable}
      onChange={(value) => {
        voice.setPartnerGain(memberId, value);
      }}
    />
  );
}

/** How loud one person's voice plays, apart from the film's volume and everyone else's (D281 §4). */
export function PersonVoiceVolume(
  props: VoiceInUse & { readonly memberId: number; readonly name: string },
): JSX.Element {
  const { t } = useTranslation();
  return (
    <label className="together-voice__partner">
      {t("together.voice.personVolume", { name: props.name })}{" "}
      <PersonVoiceRange
        voice={props.voice}
        snapshot={props.snapshot}
        tap={props.tap}
        memberId={props.memberId}
      />
    </label>
  );
}

/** Each other person's voice volume, one per person whatever their devices. */
export function VoiceVolumes(props: VoiceInUse): JSX.Element | null {
  const { t } = useTranslation();
  const people = peopleOf(props.snapshot);
  if (people.length === 0) {
    return null;
  }
  return (
    <fieldset className="together-voice__volumes">
      <legend>{t("together.voice.volumes")}</legend>
      {people.map(({ memberId, name }) => (
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
 * microphone (D63), so the controls say so, and an iPhone's first Talk warns that only they carry
 * the film and the voices together: no one is asked whether theirs are on (D336 §4). A microphone
 * that cannot open says why.
 */
export function VoiceControls(props: VoiceInUse): JSX.Element {
  const { t } = useTranslation();
  const { voice, snapshot } = props;
  const isTalking = snapshot.status === "on";
  return (
    <section className="together-voice" aria-label={t("together.voice.label")}>
      {snapshot.isHearing ? (
        <output
          className="together-voice__status"
          data-connecting={isConnecting(snapshot) ? "" : undefined}
        >
          {voiceStatusText(t, snapshot)}
        </output>
      ) : null}
      {snapshot.isHearing && props.tap.isWaiting ? (
        <button type="button" className="together-voice__tap" onClick={props.tap.play}>
          {t("together.voice.tapToHear")}
        </button>
      ) : null}
      {snapshot.failure === undefined ? null : (
        <p role="alert">{t(`together.voice.failure.${snapshot.failure}`)}</p>
      )}
      {snapshot.browserReason === undefined ? null : (
        <p className="together-voice__reason">
          {t("together.voice.browserReason", { reason: snapshot.browserReason })}
        </p>
      )}
      <TalkToggle
        voice={voice}
        snapshot={snapshot}
        tap={props.tap}
        shouldWarnFirst={movesSoundToTheEarpiece()}
      />
      {isTalking ? (
        <output className="together-voice__talking">{t("together.voice.talking")}</output>
      ) : null}
      {snapshot.status === "starting" ? <output>{t("together.voice.starting")}</output> : null}
      <OnlyOnThePanelsPage>
        <VoiceVolumes voice={voice} snapshot={snapshot} tap={props.tap} />
      </OnlyOnThePanelsPage>
      <p className="together-voice__hint">{t("together.voice.headphones")}</p>
    </section>
  );
}

/**
 * Talk among the overlay's buttons, beside 🔈, while this page is in a Watch Together session (D440
 * §3): the one toggle (D281 §7), blue with its ring following one's voice while on, grey and struck
 * through while off; that is the whole sign of its state. It shows only where the device has a
 * microphone (useHasMicrophone), so not on a television playing alone, and, while on, until it is
 * turned off. It is in the overlay, so it shows only while the overlay does (D440 §1). On the cast's
 * remote, the control centre while casting (D440 §5), it is the same toggle, drawn on the page
 * (isOverVideo false, together-on-the-remote.tsx).
 */
export function SessionTalk(props: { readonly isOverVideo: boolean }): JSX.Element | null {
  const voice = useSessionVoice();
  const hasMicrophone = useHasMicrophone();
  if (voice === undefined || (!hasMicrophone && voice.snapshot.status !== "on")) {
    return null;
  }
  return (
    <TalkToggle
      voice={voice.voice}
      snapshot={voice.snapshot}
      tap={voice.tap}
      shouldWarnFirst={movesSoundToTheEarpiece()}
      isOverVideo={props.isOverVideo}
    />
  );
}
