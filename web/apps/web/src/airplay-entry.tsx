// SPDX-License-Identifier: MPL-2.0

import "./cast-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  askAirPlayGrant,
  grantOriginOf,
  noLog,
  useAirPlayHere,
  useIsOnAirPlay,
  type AirPlayHere,
} from "./airplay.ts";
import { FindDevicesIcon } from "./icons.tsx";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/**
 * Cast's menu's AirPlay (D473), after a separator, as Chromecast is: offered only where this tab plays
 * a film that AirPlay is offered for (airPlayHere: Safari, the person's own player on its ladder) and only
 * where the page reaches the core at a name a receiver can reach too (grantOriginOf), never over
 * WebRTC alone. Its grant is asked for as the menu opens, so the press keeps its gesture for Safari's
 * picker; till the grant is had, or where it was refused, AirPlay is not pressable. Pressed, the film
 * goes to AirPlay and onPressed is told, to close the menu. While the film plays on AirPlay, it says
 * so, with AirPlay controls, which shows AirPlay's picker, where every choice is the person's: another
 * receiver, its sound, or this device, the film coming back to this page (D523 §6); it closes the menu
 * too. There is no Stop: the page cannot end AirPlay itself. Left open as the film changes, it
 * offers the new film's AirPlay, by a grant of its own. A Watch Together guest by link's Cast dialog
 * offers it too, its grant their membership's (D519 §1, together-guest-cast.tsx).
 */
export function AirPlayEntry(props: {
  readonly onPressed?: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { pathMoves } = useServices();
  const here = useAirPlayHere();
  const isOn = useIsOnAirPlay();
  const [origin] = useState(() => grantOriginOf(pathMoves, globalThis.location.origin, noLog));
  if (here === undefined || origin === undefined) {
    return null;
  }
  if (isOn) {
    return (
      <>
        <hr className="cast-to__separator" />
        <output>{t("cast.playingOnAirPlay")}</output>
        <button
          type="button"
          onClick={() => {
            here.showControls();
            props.onPressed?.();
          }}
        >
          <FindDevicesIcon />
          {t("cast.airPlayControls")}
        </button>
      </>
    );
  }
  return <AirPlayStart here={here} origin={origin} onPressed={props.onPressed} />;
}

/** AirPlay pressable once the grant asked for as it shows is had; pressed, the film goes to it. */
function AirPlayStart(props: {
  readonly here: AirPlayHere;
  readonly origin: string;
  readonly onPressed?: (() => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { here, origin } = props;
  // A grant per opening (D473): one kept from an earlier opening may be revoked by its AirPlay's end.
  const [grant] = useResource(
    `airplay-grant/${here.mediaToken}`,
    async () => await askAirPlayGrant(here, { api, log: noLog }, origin),
    { isSingleUse: true },
  );
  return (
    <>
      <hr className="cast-to__separator" />
      <button
        type="button"
        disabled={grant.state !== "loaded"}
        onClick={() => {
          if (grant.state === "loaded") {
            here.start(grant.value.url);
            props.onPressed?.();
          }
        }}
      >
        <FindDevicesIcon />
        {t("cast.airPlay")}
      </button>
    </>
  );
}
