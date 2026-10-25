// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  codecAnswersFrom,
  deviceReportFrom,
  fireTvHintOf,
  labCodecs,
  labFeatureNames,
  type DeviceFacts,
  type LabFeature,
} from "./device-report.ts";

/** Facts with every feature absent but those named. */
function factsWith(has: readonly LabFeature[], codecs: DeviceFacts["codecs"]): DeviceFacts {
  const features = Object.fromEntries(labFeatureNames.map((name) => [name, has.includes(name)]));
  return {
    userAgent: "Mozilla/5.0 (Linux; Android 11; AFTKA Build/RS8101) Silk/120.1",
    screen: { width: 1920, height: 1080, colorDepth: 24 },
    viewport: { width: 960, height: 540 },
    devicePixelRatio: 2,
    features: features as Record<LabFeature, boolean>,
    codecs,
  };
}

describe("a device's capability report", () => {
  it("asks MSE, ManagedMediaSource and the element of every codec, saying which it has not", () => {
    const answers = codecAnswersFrom({
      isTypeSupported: (type) => type.includes("avc1"),
      managedIsTypeSupported: undefined,
      canPlayType: (type) => (type.includes("ec-3") ? "probably" : ""),
    });
    expect(answers).toHaveLength(labCodecs.length);
    expect(answers.find((answer) => answer.name === "H.264 High 4.0")).toEqual({
      name: "H.264 High 4.0",
      type: 'video/mp4; codecs="avc1.640028"',
      isMseSupported: true,
      isManagedSupported: undefined,
      canPlayType: "",
    });
    expect(answers.find((answer) => answer.name === "E-AC-3")).toMatchObject({
      isMseSupported: false,
      canPlayType: "probably",
    });
  });

  it("takes a probe that throws as no answer, and goes on", () => {
    const answers = codecAnswersFrom({
      isTypeSupported: () => {
        throw new TypeError("odd type");
      },
      managedIsTypeSupported: () => true,
      canPlayType: () => {
        throw new TypeError("odd type");
      },
    });
    expect(
      answers.every(
        (answer) => answer.isMseSupported === false && answer.isManagedSupported === true,
      ),
    ).toBe(true);
    expect(answers.every((answer) => answer.canPlayType === "")).toBe(true);
  });

  it("lists what the page has and lacks, its native HLS and its Fire TV", () => {
    const report = deviceReportFrom(
      factsWith(["mediaSource", "rtcPeerConnection"], {
        isTypeSupported: () => false,
        managedIsTypeSupported: undefined,
        canPlayType: (type) => (type === "application/vnd.apple.mpegurl" ? "maybe" : ""),
      }),
    );
    expect(report.has).toEqual(["mediaSource", "rtcPeerConnection"]);
    expect(report.lacks).toHaveLength(labFeatureNames.length - 2);
    expect(report.lacks).toContain("setSinkId");
    expect(report.nativeHls).toEqual({ "application/vnd.apple.mpegurl": "maybe" });
    expect(report.fireTv).toBe("Fire OS, model AFTKA");
    expect(report.screen).toEqual({ width: 1920, height: 1080, colorDepth: 24 });
    // As JSON the lab endpoint takes: nothing is lost going through it.
    expect(JSON.parse(JSON.stringify(report))).toMatchObject({ devicePixelRatio: 2 });
  });
});

describe("a Fire TV's user agent", () => {
  it("names Fire OS by its Android and model, Vega OS by its name, and nothing else", () => {
    expect(fireTvHintOf("Mozilla/5.0 (Linux; Android 9; AFTMM Build/PS7633) Silk/98")).toBe(
      "Fire OS, model AFTMM",
    );
    expect(fireTvHintOf("Mozilla/5.0 (X11; Linux; Vega 1.1; AFTCA002) Silk/130")).toBe(
      "Vega OS, model AFTCA002",
    );
    expect(fireTvHintOf("Mozilla/5.0 (X11; Linux aarch64) Silk/130")).toBe(
      "Silk not on Android: perhaps Vega OS",
    );
    expect(
      fireTvHintOf("Mozilla/5.0 (Linux; Android 12; BRAVIA) Chrome/120 Mobile Safari/537.36"),
    ).toBeUndefined();
  });
});
