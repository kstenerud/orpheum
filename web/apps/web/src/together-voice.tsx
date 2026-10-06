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
import { useEffect, useMemo, useSyncExternalStore, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { membershipHeaders, type Membership } from "./together-storage.ts";
import { monotonicMs } from "./use-together.ts";

/** The relay's ICE servers for a membership (D243 §8), as the browser's RTCPeerConnection takes them. */
async function iceServersFor(api: Api, membership: Membership): Promise<readonly RTCIceServer[]> {
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

/** This person's voice in their session, by their membership, ended as the component goes. */
function useVoice(
  membership: Membership,
  sessionToken: string | undefined,
): { readonly voice: TogetherVoice; readonly snapshot: VoiceSnapshot } {
  const { api, openTogetherSocket, voiceDevices } = useServices();
  const devices = useMemo(() => voiceDevices(), [voiceDevices]);
  const voice = useMemo(
    () =>
      createTogetherVoice({
        credential: membership.credential,
        sessionToken,
        openSocket: openTogetherSocket,
        monotonicMs,
        fetchIceServers: () => iceServersFor(api, membership),
        devices,
      }),
    [api, membership, sessionToken, openTogetherSocket, devices],
  );
  useEffect(
    () => () => {
      voice.stop();
      devices.close();
    },
    [voice, devices],
  );
  const snapshot = useSyncExternalStore(voice.subscribe, voice.snapshot);
  return { voice, snapshot };
}

/**
 * Where voice stands, in a line: connecting, alone, or who it carries voice to (watch-together.md:
 * What each person sees).
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

/** While voice is on: who it reaches, mute, leave, and the partners' volume. */
function VoiceOn(props: {
  readonly voice: TogetherVoice;
  readonly snapshot: VoiceSnapshot;
}): JSX.Element {
  const { t } = useTranslation();
  const { voice, snapshot } = props;
  return (
    <>
      <output className="together-voice__status">{voiceStatusText(t, snapshot)}</output>
      <div className="together-voice__buttons">
        <button
          type="button"
          aria-pressed={snapshot.isMuted}
          onClick={() => {
            voice.setMuted(!snapshot.isMuted);
          }}
        >
          {snapshot.isMuted ? t("together.voice.unmute") : t("together.voice.mute")}
        </button>
        <button
          type="button"
          onClick={() => {
            voice.stop();
          }}
        >
          {t("together.voice.leave")}
        </button>
      </div>
      <label className="together-voice__partner">
        {t("together.voice.partner")}{" "}
        <input
          type="range"
          min={0}
          max={partnerGainMost * 100}
          value={Math.round(snapshot.partnerGain * 100)}
          onChange={(event) => {
            voice.setPartnerGain(Number(event.target.value) / 100);
          }}
        />
      </label>
    </>
  );
}

/**
 * Voice in a Watch Together session, for the host and every viewer (watch-together.md, Voice):
 * Talk turns it on, from the press the microphone needs; then mute, leave, and the partners'
 * volume, apart from the film's. Headphones are the supported way to keep the film out of the
 * microphone (D63), so the controls say so. A microphone that cannot open says why.
 */
export function VoiceControls(props: {
  readonly membership: Membership;
  /** A signed-in person's session token, sent in the voice device's hello too (D243 §2). */
  readonly sessionToken?: string | undefined;
  /** Whether the browser's processing works on the microphone: the session's tuning says. */
  readonly isMicrophoneProcessed: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { voice, snapshot } = useVoice(props.membership, props.sessionToken);
  const isOn = snapshot.status === "on";
  return (
    <section className="together-voice" aria-label={t("together.voice.label")}>
      {isOn ? null : (
        <button
          type="button"
          disabled={snapshot.status === "starting"}
          onClick={() => {
            void voice.start(props.isMicrophoneProcessed);
          }}
        >
          {t("together.voice.talk")}
        </button>
      )}
      {snapshot.status === "starting" ? <output>{t("together.voice.starting")}</output> : null}
      {snapshot.failure === undefined ? null : (
        <p role="alert">{t(`together.voice.failure.${snapshot.failure}`)}</p>
      )}
      {snapshot.browserReason === undefined ? null : (
        <p className="together-voice__reason">
          {t("together.voice.browserReason", { reason: snapshot.browserReason })}
        </p>
      )}
      {isOn ? <VoiceOn voice={voice} snapshot={snapshot} /> : null}
      <p className="together-voice__hint">{t("together.voice.headphones")}</p>
    </section>
  );
}
