// SPDX-License-Identifier: MPL-2.0

import "./lab-words.ts";

import type { TFunction } from "i18next";
import {
  browserDeviceFacts,
  countAudioOutputs,
  deviceReportFrom,
  failureText,
  measureAudioLatency,
  testLoopback,
  toneTrack,
  type AudioLatency,
  type DeviceReport,
  type LoopbackResult,
} from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { LabFrame, LabSendButton, yesNo } from "./lab-frame.tsx";
import { useLabSending } from "./lab-send.ts";
import { useServices } from "./services.tsx";

/** What the press-started tests found: the audio context's delay, the outputs, the loopback. */
interface PressedTests {
  readonly audioLatency: AudioLatency | string;
  readonly audioOutputs: number | string;
  readonly loopback: LoopbackResult | string;
}

/** How loud the call's tone plays out: barely, since only its arrival is measured. */
const loopbackVolume = 0.05;

/** Calls the page with itself carrying a tone, played out quietly by an element of its own. */
async function loopbackWithTone(): Promise<LoopbackResult | string> {
  if (!("RTCPeerConnection" in globalThis) || !("AudioContext" in globalThis)) {
    return "no RTCPeerConnection or AudioContext";
  }
  const context = new AudioContext();
  const playOut = new Audio();
  playOut.volume = loopbackVolume;
  try {
    await context.resume();
    const tone = toneTrack(context);
    try {
      return await testLoopback(tone.track, (received) => {
        playOut.srcObject = received ?? null;
        void playOut.play().catch(() => undefined);
      });
    } finally {
      tone.stop();
    }
  } catch (cause) {
    return failureText(cause);
  } finally {
    await context.close().catch(() => undefined);
  }
}

/** The tests a press starts, an audio context and a call needing one. */
async function runPressedTests(): Promise<PressedTests> {
  const audioLatency = (await measureAudioLatency().catch(failureText)) ?? "no AudioContext";
  const audioOutputs = await countAudioOutputs();
  return { audioLatency, audioOutputs, loopback: await loopbackWithTone() };
}

/** The audio context's delay, in words. */
function latencyText(
  t: ReturnType<typeof useTranslation>["t"],
  latency: AudioLatency | string,
): string {
  return typeof latency === "string"
    ? latency
    : t("lab.device.latencies", {
        base: String(latency.baseLatencyMs ?? "?"),
        output: String(latency.outputLatencyMs ?? "?"),
      });
}

/** The call with itself, in words. */
function loopbackText(
  t: ReturnType<typeof useTranslation>["t"],
  loopback: LoopbackResult | string,
): string {
  if (typeof loopback === "string") {
    return loopback;
  }
  const isHeard = loopback.levels.some((level) => level !== null && level > 0);
  return (
    loopback.failure ??
    t("lab.device.loopback", {
      connect: String(loopback.connectMs ?? "?"),
      trips: loopback.roundTripsMs.join(", "),
      heard: yesNo(t, isHeard),
    })
  );
}

/** What the press's tests found, once they have run. */
function PressedResults(props: { readonly tests: PressedTests }): JSX.Element {
  const { t } = useTranslation();
  return (
    <dl className="lab__facts lab__big">
      <dt>{t("lab.device.audioLatency")}</dt>
      <dd>{latencyText(t, props.tests.audioLatency)}</dd>
      <dt>{t("lab.device.outputs")}</dt>
      <dd>{String(props.tests.audioOutputs)}</dd>
      <dt>{t("lab.device.webrtc")}</dt>
      <dd>{loopbackText(t, props.tests.loopback)}</dd>
    </dl>
  );
}

/** The device's own facts: its Fire TV, its screen and page, native HLS. */
function DeviceFactsList(props: { readonly report: DeviceReport }): JSX.Element {
  const { t } = useTranslation();
  const { report } = props;
  return (
    <dl className="lab__facts lab__big">
      <dt>{t("lab.device.fireTv")}</dt>
      <dd>{report.fireTv ?? t("lab.device.notFireTv")}</dd>
      <dt>{t("lab.device.screen")}</dt>
      <dd>
        {t("lab.device.size", {
          width: String(report.screen.width),
          height: String(report.screen.height),
          ratio: String(report.devicePixelRatio),
        })}
      </dd>
      <dt>{t("lab.device.viewport")}</dt>
      <dd>
        {t("lab.device.pixels", {
          width: String(report.viewport.width),
          height: String(report.viewport.height),
        })}
      </dd>
      <dt>{t("lab.device.display")}</dt>
      <dd>{displayText(t, report)}</dd>
      <dt>{t("lab.device.nativeHls")}</dt>
      <dd>{yesNo(t, Object.keys(report.nativeHls).length > 0)}</dd>
    </dl>
  );
}

/** What the screen's media queries said: high dynamic range for the page and video, and colours. */
function displayText(t: TFunction, report: DeviceReport): string {
  const said = (answer: string | undefined): string => answer ?? t("lab.device.unanswered");
  return t("lab.device.displayAnswer", {
    page: said(report.display?.dynamicRange),
    video: said(report.display?.videoDynamicRange),
    gamut: said(report.display?.colorGamut),
  });
}

/** What MSE, ManagedMediaSource and the element say of each codec. */
function CodecTable(props: { readonly report: DeviceReport }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="lab__scroll">
      <table className="lab__table">
        <thead>
          <tr>
            <th scope="col">{t("lab.device.codec")}</th>
            <th scope="col">{t("lab.device.mse")}</th>
            <th scope="col">{t("lab.device.managed")}</th>
            <th scope="col">{t("lab.device.element")}</th>
          </tr>
        </thead>
        <tbody>
          {props.report.codecs.map((codec) => (
            <tr key={codec.type}>
              <th scope="row">{codec.name}</th>
              <td>{yesNo(t, codec.isMseSupported)}</td>
              <td>{yesNo(t, codec.isManagedSupported)}</td>
              <td>{codec.canPlayType === "" ? t("lab.no") : codec.canPlayType}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/**
 * The device report (plans/tv-device-tests.md): what the browser says of itself, what it decodes,
 * which APIs it has, then, after one press (an audio context and a call need one), its audio delay,
 * its outputs and a WebRTC call with itself. Sent to the server when the press's tests end.
 */
export function LabDevice(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [report] = useState<DeviceReport>(() => deviceReportFrom(browserDeviceFacts()));
  const [tests, setTests] = useState<PressedTests | undefined>();
  const [isRunning, setIsRunning] = useState(false);
  const { sending, send } = useLabSending(api, "device");
  const run = (): void => {
    setIsRunning(true);
    void runPressedTests().then((found) => {
      setIsRunning(false);
      setTests(found);
      send({ ...report, ...found });
    });
  };
  return (
    <LabFrame id="lab-device" heading={t("lab.device.heading")}>
      <p className="lab__lead">{t("lab.device.explain")}</p>
      <div className="lab__actions">
        <button type="button" className="lab__button" disabled={isRunning} onClick={run}>
          {isRunning ? t("lab.device.running") : t("lab.device.run")}
        </button>
        <LabSendButton
          sending={sending}
          onSend={() => {
            send({ ...report, ...tests });
          }}
        />
      </div>
      <DeviceFactsList report={report} />
      {tests === undefined ? null : <PressedResults tests={tests} />}
      <h2>{t("lab.device.has")}</h2>
      <p className="lab__big">{report.has.join(", ")}</p>
      <h2>{t("lab.device.lacks")}</h2>
      <p className="lab__big">{report.lacks.join(", ")}</p>
      <h2>{t("lab.device.codecs")}</h2>
      <CodecTable report={report} />
      <h2>{t("lab.device.userAgent")}</h2>
      <p className="lab__mono">{report.userAgent}</p>
    </LabFrame>
  );
}
