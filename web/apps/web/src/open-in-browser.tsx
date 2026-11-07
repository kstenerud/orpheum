// SPDX-License-Identifier: MPL-2.0

/**
 * What a page whose browser runs no service worker says (the gate's noWorker phase): most often the
 * link was opened inside another app, whose built-in browser cannot play from the server, and the
 * way on is the phone's own browser. A guest in Berlin opened her link from Telegram, was told only
 * that "this browser" could not play, and got in when she tried Safari herself (2026-11-02).
 */

import { useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { copyToClipboard } from "./clipboard.ts";
import { CredentialsForm } from "./credentials-form.tsx";

/** The browser a phone opens links in: Safari on an iPhone or iPad, Chrome on Android. */
export type OwnBrowser = "safari" | "chrome" | "other";

/** Where a page runs, as its user agent tells: inside another app or not, and which. */
export interface BrowserPlace {
  /** Whether the page runs in a browser built into another app. */
  readonly isInApp: boolean;
  /** The app, where its user agent names it. */
  readonly app: string | undefined;
  /** The browser to open the link in instead. */
  readonly ownBrowser: OwnBrowser;
}

/**
 * Apps that open links in a browser of their own and name themselves in its user agent, each with
 * its name as a person knows it. An app not named here is still found by its web view (inAppOf).
 */
const appsByUserAgent: readonly (readonly [RegExp, string])[] = [
  [/Telegram/i, "Telegram"],
  [/WhatsApp/i, "WhatsApp"],
  [/Instagram/, "Instagram"],
  [/FBAN|FBAV|FB_IAB|FBIOS/, "Facebook"],
  [/\bLine\//, "LINE"],
  [/Snapchat/i, "Snapchat"],
  [/BytedanceWebview|musical_ly|TikTok/i, "TikTok"],
  [/LinkedInApp/i, "LinkedIn"],
  [/\bGSA\//, "the Google app"],
];

/**
 * Where a page runs, from its user agent. On an iPhone or iPad a web view has no "Safari/" token
 * where Safari, Chrome and Firefox there all have one; on Android a web view says "; wv)".
 */
export function browserPlaceOf(userAgent: string): BrowserPlace {
  const isApple = /iPhone|iPad|iPod/.test(userAgent);
  const isAndroid = userAgent.includes("Android");
  const app = appsByUserAgent.find(([pattern]) => pattern.test(userAgent))?.[1];
  const isWebView = isApple
    ? !userAgent.includes("Safari/")
    : isAndroid && userAgent.includes("; wv)");
  return {
    isInApp: app !== undefined || isWebView,
    app,
    ownBrowser: ownBrowserOf(isApple, isAndroid),
  };
}

/** The browser a phone opens links in: Safari on Apple's, Chrome on Android, else none named. */
function ownBrowserOf(isApple: boolean, isAndroid: boolean): OwnBrowser {
  if (isApple) {
    return "safari";
  }
  return isAndroid ? "chrome" : "other";
}

/**
 * The page saying to open the link in the phone's own browser, naming the app it opened in where the
 * browser tells, with the link to copy and paste there; a copy the app refuses leaves the link shown
 * to select. A browser not inside an app, as a private window, is told to use an ordinary window.
 */
export function OpenInBrowser(props: {
  readonly address?: string;
  readonly userAgent?: string;
  readonly copy?: (text: string) => Promise<void>;
  /** What comes below the page's form, as what happened to copy (CredentialsForm's after). */
  readonly after?: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const [copied, setCopied] = useState<"copied" | "refused" | undefined>(undefined);
  const address = props.address ?? globalThis.location.href;
  const copy = props.copy ?? copyToClipboard;
  const place = browserPlaceOf(props.userAgent ?? globalThis.navigator.userAgent);
  if (!place.isInApp) {
    return (
      <CredentialsForm
        heading={t("discovery.noWorker.heading")}
        onSubmit={() => undefined}
        after={props.after}
      >
        <output>{t("discovery.noWorker.text")}</output>
      </CredentialsForm>
    );
  }
  const browser = t(`discovery.noWorker.browser.${place.ownBrowser}`);
  return (
    <CredentialsForm
      heading={t("discovery.noWorker.inApp.heading", { browser })}
      onSubmit={() => undefined}
      after={props.after}
    >
      <output>
        {place.app === undefined
          ? t("discovery.noWorker.inApp.someApp")
          : t("discovery.noWorker.inApp.namedApp", { app: place.app })}
      </output>
      <p>{t("discovery.noWorker.inApp.how", { browser })}</p>
      <button
        type="button"
        onClick={() => {
          copy(address).then(
            () => {
              setCopied("copied");
            },
            () => {
              setCopied("refused");
            },
          );
        }}
      >
        {t("discovery.noWorker.inApp.copy")}
      </button>
      {copied === undefined ? null : (
        <output>{t(`discovery.noWorker.inApp.${copied}`, { browser })}</output>
      )}
      {copied === "refused" ? (
        <input
          type="text"
          readOnly
          value={address}
          aria-label={t("discovery.noWorker.inApp.link")}
        />
      ) : null}
      {place.ownBrowser === "safari" ? <p>{t("discovery.noWorker.inApp.menu")}</p> : null}
    </CredentialsForm>
  );
}
