// SPDX-License-Identifier: MPL-2.0

/** How this client names itself when it signs in (D217 §8): the person can rename it later. */
export interface NewDevice {
  readonly name: string;
  readonly platform: "web";
}

/** The browser's name, from its user agent: the first that matches, so Edge is not Chrome. */
function browserOf(userAgent: string): string | undefined {
  const browsers: readonly (readonly [RegExp, string])[] = [
    [/\bEdg(A|iOS)?\//, "Edge"],
    [/\b(Firefox|FxiOS)\//, "Firefox"],
    [/\bSamsungBrowser\//, "Samsung Internet"],
    [/\b(Chrome|CriOS)\//, "Chrome"],
    [/\bSafari\//, "Safari"],
  ];
  return browsers.find(([pattern]) => pattern.test(userAgent))?.[1];
}

/** The system's name, from the user agent: Android before Linux, iPhone before Mac. */
function systemOf(userAgent: string): string | undefined {
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
