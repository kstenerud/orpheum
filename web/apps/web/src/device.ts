// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";

import { isFireTv } from "./tv-device.ts";

/**
 * How this client names itself when it signs in (D217 §8): the person can rename it later. Its
 * fields are the API's own, so a renamed field fails to compile. previousDeviceId is the device
 * this browser was signed in as before, which the server resumes (D221).
 */
export type NewDevice = components["schemas"]["NewDeviceBody"] & { readonly platform: "web" };

/**
 * The browser's name, from its user agent: the first that matches, so Edge is not Chrome, nor Silk,
 * whose agent says "like Chrome".
 */
function browserOf(userAgent: string): string | undefined {
  const browsers: readonly (readonly [RegExp, string])[] = [
    [/\bEdg(A|iOS)?\//, "Edge"],
    [/\bSilk\//, "Silk"],
    [/\b(Firefox|FxiOS)\//, "Firefox"],
    [/\bSamsungBrowser\//, "Samsung Internet"],
    [/\b(HeadlessChrome|Chrome|CriOS)\//, "Chrome"],
    [/\bSafari\//, "Safari"],
  ];
  return browsers.find(([pattern]) => pattern.test(userAgent))?.[1];
}

/**
 * The system's name, from the user agent: a Fire TV by its model code before Android, whose Fire OS
 * is (staging's device 33, a Fire TV, was "Chrome on Android"), Android before Linux, iPhone before
 * Mac.
 */
function systemOf(userAgent: string): string | undefined {
  if (isFireTv(userAgent)) {
    return "Fire TV";
  }
  const systems: readonly (readonly [RegExp, string])[] = [
    [/\bAndroid\b/, "Android"],
    [/\biPhone\b/, "iPhone"],
    [/\biPad\b/, "iPad"],
    [/\bCrOS\b/, "ChromeOS"],
    [/\bWindows\b/, "Windows"],
    [/\bMac OS X\b/, "Mac"],
    [/\bLinux\b/, "Linux"],
  ];
  return systems.find(([pattern]) => pattern.test(userAgent))?.[1];
}

/** This browser as a device: "Chrome on Android", or as much of that as the user agent says. */
export function deviceFrom(userAgent: string): NewDevice {
  const browser = browserOf(userAgent) ?? "A web browser";
  const system = systemOf(userAgent);
  return { name: system === undefined ? browser : `${browser} on ${system}`, platform: "web" };
}

/**
 * This browser as a device to sign in on: named from its user agent, resuming the last (D221) with
 * the proof that it was it, without which the server makes a new device.
 */
export function thisDevice(session: {
  readonly deviceId: () => number | undefined;
  readonly deviceProof: () => string | undefined;
}): NewDevice {
  const previousDeviceId = session.deviceId();
  const previousDeviceProof = session.deviceProof();
  const named = deviceFrom(navigator.userAgent);
  if (previousDeviceId === undefined) {
    return named;
  }
  return previousDeviceProof === undefined
    ? { ...named, previousDeviceId }
    : { ...named, previousDeviceId, previousDeviceProof };
}
