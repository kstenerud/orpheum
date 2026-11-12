// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  codecAnswersFrom,
  deviceReportFrom,
  fireTvHintOf,
  isFireTv,
  labCodecs,
  labFeatureNames,
  mediaQueryAnswer,
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

describe("a screen's media queries", () => {
  it("answer the first value that matches, and nothing where none does (the owner's TCL TV, 2026-11-06)", () => {
    const hdrVideo = (query: string): boolean =>
      query === "(video-dynamic-range: high)" || query === "(color-gamut: p3)";
    expect(mediaQueryAnswer(hdrVideo, "video-dynamic-range", ["high", "standard"])).toBe("high");
    expect(mediaQueryAnswer(hdrVideo, "color-gamut", ["rec2020", "p3", "srgb"])).toBe("p3");
    expect(mediaQueryAnswer(hdrVideo, "dynamic-range", ["high", "standard"])).toBeUndefined();
    const throws = (): boolean => {
      throw new TypeError("no matchMedia");
    };
    expect(mediaQueryAnswer(throws, "dynamic-range", ["high", "standard"])).toBeUndefined();
  });

  it("go into the report as the page read them", () => {
    const display = {
      dynamicRange: "standard",
      videoDynamicRange: "high",
      colorGamut: "p3",
    } as const;
    const facts = factsWith([], {
      isTypeSupported: undefined,
      managedIsTypeSupported: undefined,
      canPlayType: () => "",
    });
    expect(deviceReportFrom({ ...facts, display }).display).toEqual(display);
  });

  it("include DTS and TrueHD among the codecs asked", () => {
    expect(labCodecs.map(([name]) => name)).toEqual(
      expect.arrayContaining(["DTS", "DTS-HD", "DTS:X", "Dolby TrueHD", "AC-4", "Dolby Vision 5"]),
    );
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

  it("is a Fire TV by one model code pattern, which the hint names it by too", () => {
    const agents = [
      "Mozilla/5.0 (Linux; Android 9; AFTSSS Build/PS7633) Silk/138",
      "Mozilla/5.0 (Linux; Android 11; AFTKRT0123 Build/RS8101) Silk/138",
      "Mozilla/5.0 (Linux; Android 12; BRAVIA) Chrome/120 Mobile Safari/537.36",
    ];
    expect(agents.map((agent) => [isFireTv(agent), fireTvHintOf(agent)])).toEqual([
      [true, "Fire OS, model AFTSSS"],
      [true, "Fire OS, model AFTKRT0123"],
      [false, undefined],
    ]);
  });
});
