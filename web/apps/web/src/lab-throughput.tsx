// SPDX-License-Identifier: MPL-2.0

import "./lab-words.ts";

import { apiPrefix, failureText, timeLoad, type LoadTiming } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage } from "./browser-storage.ts";
import { LabFrame, LabSendButton } from "./lab-frame.tsx";
import { useLabSending } from "./lab-send.ts";
import { useServices, type Services } from "./services.tsx";

/** Where the core's LAN address, as last typed on this device, is kept (client-keys.ts). */
const lanAddressKey = "orpheum.lab.lanAddress";

/**
 * How much each load fetches: 32 MiB, eight seconds at the Fire TV's 30 Mbit/s and a second at
 * 270, long enough that a connection's start counts little. A justified constant, within the core's
 * bounds for the body (lab_throughput.go).
 */
export const throughputBytes = 32 * 1024 * 1024;

/**
 * The ways the page loads the body (webrtc-throughput-2026-10-28.md): its own requests' path, as the
 * API's calls go (WebRTC's data channel, or HTTPS); the browser's own load, which the service worker
 * hands to the page's transport while the page is on WebRTC, as a film's chunks go; and the core's
 * LAN name over HTTPS, when one is typed.
 */
export const throughputLegs = ["transport", "relay", "lan"] as const;

/** One way of loading the body. */
export type ThroughputLeg = (typeof throughputLegs)[number];

/** What one leg found: its timing and the path the core says it came over, or why it failed. */
export type LegResult =
  | (LoadTiming & { readonly over: string })
  | { readonly failure: string }
  | { readonly isSkipped: true };

/** The body's address under the API, for leg. */
function bodyPath(leg: ThroughputLeg): string {
  return `/lab/throughput?bytes=${String(throughputBytes)}&leg=${leg}`;
}

/** The headers each load is signed in with: the session's token, which the core's gate needs. */
function signedIn(token: string | undefined): Headers {
  const headers = new Headers();
  if (token !== undefined) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  return headers;
}

/** Loads the body by leg, through services; resolves with its response. */
function loadBy(
  services: Pick<Services, "api" | "fetchMedia" | "session">,
  leg: ThroughputLeg,
  lanAddress: string,
): Promise<Response> {
  const headers = signedIn(services.session.token());
  switch (leg) {
    case "transport":
      return services.api
        .GET("/lab/throughput", {
          params: { query: { bytes: throughputBytes, leg } },
          parseAs: "stream",
        })
        .then(({ response }) => response);
    case "relay":
      return services.fetchMedia(`${apiPrefix}${bodyPath(leg)}`, { headers });
    case "lan":
      return services.fetchMedia(`${new URL(lanAddress).origin}${apiPrefix}${bodyPath(leg)}`, {
        headers,
        mode: "cors",
        credentials: "omit",
      });
  }
}

/** Times one leg, its failure in words; a LAN leg with no address is skipped. */
async function timeLeg(
  services: Pick<Services, "api" | "fetchMedia" | "session">,
  leg: ThroughputLeg,
  lanAddress: string,
): Promise<LegResult> {
  if (leg === "lan" && lanAddress === "") {
    return { isSkipped: true };
  }
  let over = "";
  try {
    const timing = await timeLoad(async () => {
      const response = await loadBy(services, leg, lanAddress);
      over = response.headers.get("Orpheum-Over") ?? "";
      return response;
    });
    return { ...timing, over };
  } catch (cause) {
    return { failure: failureText(cause) };
  }
}

/** Times each leg in turn, never two at once, telling onLeg as each ends. */
export async function runThroughputTest(
  services: Pick<Services, "api" | "fetchMedia" | "session">,
  lanAddress: string,
  onLeg: (leg: ThroughputLeg, result: LegResult) => void,
): Promise<Record<ThroughputLeg, LegResult>> {
  const results: Partial<Record<ThroughputLeg, LegResult>> = {};
  for (const leg of throughputLegs) {
    const result = await timeLeg(services, leg, lanAddress);
    results[leg] = result;
    onLeg(leg, result);
  }
  return {
    transport: { isSkipped: true },
    relay: { isSkipped: true },
    lan: { isSkipped: true },
    ...results,
  };
}

/** A leg's result, in words. */
function resultText(
  t: ReturnType<typeof useTranslation>["t"],
  result: LegResult | undefined,
): string {
  if (result === undefined) {
    return t("lab.throughput.notRun");
  }
  if ("isSkipped" in result) {
    return t("lab.throughput.skipped");
  }
  if ("failure" in result) {
    return t("lab.throughput.failed", { why: result.failure });
  }
  return t("lab.throughput.result", {
    rate: String(result.megabitsPerSecond),
    seconds: (result.totalMs / 1000).toFixed(1),
    firstByte: String(result.firstByteMs),
    over: result.over === "" ? "?" : result.over,
  });
}

/**
 * The throughput test (webrtc-throughput-2026-10-28.md): one press loads the core's test body each
 * way the page can, one after another, and shows each one's rate, then sends them to the server's log
 * with the browser's user agent. Off the playback path: it plays nothing and changes nothing.
 */
export function LabThroughput(): JSX.Element {
  const { t } = useTranslation();
  const services = useServices();
  const [lanAddress, setLanAddress] = useState(() => browserStorage().getItem(lanAddressKey) ?? "");
  const [results, setResults] = useState<Partial<Record<ThroughputLeg, LegResult>>>({});
  const [isRunning, setIsRunning] = useState(false);
  const { sending, send } = useLabSending(services.api, "throughput");
  const report = (found: Partial<Record<ThroughputLeg, LegResult>>): Record<string, unknown> => ({
    bytes: throughputBytes,
    ...found,
    userAgent: navigator.userAgent,
  });
  const run = (): void => {
    setIsRunning(true);
    setResults({});
    void runThroughputTest(services, lanAddress, (leg, result) => {
      setResults((before) => ({ ...before, [leg]: result }));
    }).then((found) => {
      setIsRunning(false);
      send(report(found));
    });
  };
  return (
    <LabFrame id="lab-throughput" heading={t("lab.throughput.heading")}>
      <p className="lab__lead">{t("lab.throughput.explain")}</p>
      <label className="lab__big lab__field">
        {t("lab.throughput.lanAddress")}
        <input
          type="url"
          value={lanAddress}
          placeholder={t("lab.throughput.lanAddressExample")}
          onChange={(event) => {
            const typed = event.target.value.trim();
            setLanAddress(typed);
            browserStorage().setItem(lanAddressKey, typed);
          }}
        />
      </label>
      <div className="lab__actions">
        <button type="button" className="lab__button" disabled={isRunning} onClick={run}>
          {isRunning ? t("lab.throughput.running") : t("lab.throughput.run")}
        </button>
        <LabSendButton
          sending={sending}
          onSend={() => {
            send(report(results));
          }}
        />
      </div>
      <dl className="lab__facts lab__big">
        {throughputLegs.map((leg) => (
          <div key={leg} className="lab__fact">
            <dt>{t(`lab.throughput.leg.${leg}`)}</dt>
            <dd>{resultText(t, results[leg])}</dd>
          </div>
        ))}
      </dl>
    </LabFrame>
  );
}
