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
  useSyncExternalStore,
  type JSX,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";

import type { AudioOutput } from "./accounts.ts";
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

/**
 * Talk, one toggle (the owner, 2026-10-11): a press opens the microphone, from the press it needs,
 * and the next closes it, still hearing everyone. Its look is provisional until the owner chooses
 * one (talk-toggle-options).
 */
function TalkToggle(props: VoiceInUse): JSX.Element {
  const { t } = useTranslation();
  const { voice, snapshot } = props;
  const isTalking = snapshot.status === "on";
  return (
    <button
      type="button"
      className="together-voice__talk"
      aria-pressed={isTalking}
      disabled={snapshot.status === "starting"}
      onClick={() => {
        if (isTalking) {
          voice.stop();
        } else {
          void voice.start({
            shouldCancelEcho: snapshot.tuning.shouldCancelMicrophoneEcho,
            shouldLevel: snapshot.tuning.shouldBrowserLevelMicrophone,
          });
        }
      }}
    >
      {isTalking ? t("together.voice.stopTalking") : t("together.voice.talk")}
    </button>
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
      <TalkToggle voice={voice} snapshot={snapshot} />
      {isTalking ? (
        <output className="together-voice__talking">{t("together.voice.talking")}</output>
      ) : null}
      {snapshot.status === "starting" ? <output>{t("together.voice.starting")}</output> : null}
      {!isTalking && props.audioOutput !== "headphones" && movesSoundToTheEarpiece() ? (
        <p className="together-voice__warning">{t("together.voice.earpiece")}</p>
      ) : null}
      <VoiceVolumes voice={voice} snapshot={snapshot} />
      <p className="together-voice__hint">{t("together.voice.headphones")}</p>
    </section>
  );
}
