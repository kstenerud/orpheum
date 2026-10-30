// SPDX-License-Identifier: MPL-2.0

import { isPageWorkerBypassed, reportFailure } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { pageWorkerOf, type PageWorker } from "./screen-load.tsx";

/**
 * The page's one probe of its worker, made once it is first asked for: a page under a discovery's
 * loader that a worker controls asks it for an address only the worker answers
 * (isPageWorkerBypassed); answered by the network, or not at all, the worker is bypassed, as DevTools'
 * "Bypass for network" does, and the core's log is told, once. Elsewhere there is nothing to ask: at a
 * core's own origin media goes to the network anyway, and a page no worker controls has none to
 * bypass (screen-load.tsx says so).
 */
let probed: Promise<boolean> | undefined;

/** Whether this page's worker is bypassed, probed once for the page's life. */
export function isThisPageWorkerBypassed(): Promise<boolean> {
  probed ??= probeWorker(pageWorkerOf(window), isPageWorkerBypassed);
  return probed;
}

/**
 * Whether worker, the page's, is bypassed, by isBypassed's probe, asked only under a loader of a
 * page a worker controls; the core's log told when it is.
 */
export async function probeWorker(
  worker: PageWorker,
  isBypassed: () => Promise<boolean>,
): Promise<boolean> {
  if (!worker.isUnderLoader || !worker.isControlled) {
    return false;
  }
  const isWorkerBypassedNow = await isBypassed();
  if (isWorkerBypassedNow) {
    reportFailure(
      new Error(
        "developer tools bypass the page's service worker: its probe was answered by the network, or not at all, so its posters and films do not load (DevTools' Bypass for network)",
      ),
    );
  }
  return isWorkerBypassedNow;
}

/**
 * The line above every screen while developer tools bypass the page's worker: posters and films 404
 * at a client origin with nothing said, and an afternoon went on the worker before the box was found
 * ticked (idiosyncrasies.md, the Bypass entry; agent-failures A51). probe is the page's own unless a
 * test gives one.
 */
export function WorkerBypassedNote(props: {
  readonly probe?: () => Promise<boolean>;
}): JSX.Element | null {
  const { t } = useTranslation();
  const [isBypassed, setIsBypassed] = useState(false);
  const probe = props.probe ?? isThisPageWorkerBypassed;
  useEffect(() => {
    let isShown = true;
    probe().then(
      (isWorkerBypassedNow) => {
        if (isShown) {
          setIsBypassed(isWorkerBypassedNow);
        }
      },
      () => undefined, // a probe that cannot run says nothing: the page goes on as it was
    );
    return () => {
      isShown = false;
    };
  }, [probe]);
  return isBypassed ? (
    <p className="app-shell__note" role="alert">
      {t("discovery.workerBypassed")}
    </p>
  ) : null;
}
