// SPDX-License-Identifier: MPL-2.0

import { isPageWorkerBypassed, reportFailure } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { pageWorkerOf, type PageWorker } from "./screen-load.tsx";

/**
 * The page's probe of its worker: a page under a discovery's loader that a worker controls asks it
 * for an address only the worker answers (isPageWorkerBypassed); answered by the network, or not at
 * all, the worker is bypassed, as DevTools' "Bypass for network" does, and the core's log is told,
 * once. It probes as the note is first shown, and again each time the page's art fails to load
 * (suspectWorker): a box ticked after the page loaded found nothing at the first probe, and its
 * posters 404'd with nothing said (agent-failures A51; the owner, 2026-10-30). Elsewhere there is
 * nothing to ask: at a core's own origin media goes to the network anyway, and a page no worker
 * controls has none to bypass (screen-load.tsx says so).
 */
let probing: Promise<boolean> | undefined;
let isReported = false;
const suspicionListeners = new Set<() => void>();

/** Whether this page's worker is bypassed: one probe at a time, its answer shared by every ask. */
export function isThisPageWorkerBypassed(): Promise<boolean> {
  probing ??= probeWorker(pageWorkerOf(window), isPageWorkerBypassed).finally(() => {
    probing = undefined;
  });
  return probing;
}

/**
 * Says the page's art failed to load, so the note probes its worker again: the many posters of a page
 * failing at once share the one probe made for the first.
 */
export function suspectWorker(): void {
  for (const listener of suspicionListeners) {
    listener();
  }
}

/** Calls listener each time the page's art fails to load, until the returned stop. */
export function onWorkerSuspected(listener: () => void): () => void {
  suspicionListeners.add(listener);
  return () => {
    suspicionListeners.delete(listener);
  };
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
  if (isWorkerBypassedNow && !isReported) {
    isReported = true;
    reportFailure(
      new Error(
        "developer tools bypass the page's service worker: its probe was answered by the network, or not at all, so its posters and films do not load (DevTools' Bypass for network)",
      ),
    );
  }
  return isWorkerBypassedNow;
}

/**
 * The line above every screen while developer tools bypass the page's worker, asked again each time
 * the page's art fails to load: posters and films 404 at a client origin with nothing said, and an afternoon went on the worker before the box was found
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
    const ask = (): void => {
      probe().then(
        (isWorkerBypassedNow) => {
          if (isShown) {
            setIsBypassed(isWorkerBypassedNow);
          }
        },
        () => undefined, // a probe that cannot run says nothing: the page goes on as it was
      );
    };
    ask();
    const stop = onWorkerSuspected(ask);
    return () => {
      isShown = false;
      stop();
    };
  }, [probe]);
  return isBypassed ? (
    <p className="app-shell__note" role="alert">
      {t("discovery.workerBypassed")}
    </p>
  ) : null;
}
