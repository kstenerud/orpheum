// SPDX-License-Identifier: MPL-2.0

import { createContext, useContext, useEffect } from "react";

import { useServices } from "./services.tsx";

/**
 * How often, in seconds, a shown film's or series' page says so: the server's
 * interest_said_every_seconds, its starting value until the client settings are read (D324 §1).
 */
export const InterestSaidEveryContext = createContext(15);

/**
 * Says the server's work for itemId comes first while its page is shown (D324 §1): at once, again
 * every interest_said_every_seconds while the tab is visible, and that it left when the page goes,
 * the tab hides or the page unloads, the request kept alive past the page. A series' page that
 * shows one season says which, whose episodes the server then raises (the owner, 2026-10-24).
 */
export function usePageInterest(itemId: number, season?: number): void {
  const { api } = useServices();
  const everySeconds = useContext(InterestSaidEveryContext);
  useEffect(() => {
    const thisItem = {
      path: { id: itemId },
      ...(season === undefined ? {} : { query: { season } }),
    };
    const say = (): void => {
      if (document.visibilityState === "visible") {
        void api.PUT("/interest/{id}", { params: thisItem }).catch(() => undefined);
      }
    };
    const leave = (): void => {
      void api
        .DELETE("/interest/{id}", { params: thisItem, keepalive: true })
        .catch(() => undefined);
    };
    const onVisibility = (): void => {
      if (document.visibilityState === "visible") {
        say();
      } else {
        leave();
      }
    };
    say();
    const timer = setInterval(say, everySeconds * 1000);
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("pagehide", leave);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("pagehide", leave);
      leave();
    };
  }, [api, itemId, season, everySeconds]);
}
