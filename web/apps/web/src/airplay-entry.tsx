// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  airPlayHere,
  grantOriginOf,
  makeAirPlayGrant,
  noLog,
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
 * goes to AirPlay and onStarted is told, to close the menu. While the film plays on AirPlay, Stop
 * AirPlay is offered instead, which hands it back to this page.
 */
export function AirPlayEntry(props: {
  readonly onStarted?: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { pathMoves } = useServices();
  const [here] = useState(airPlayHere);
  const [origin] = useState(() => grantOriginOf(pathMoves, globalThis.location.origin, noLog));
  const [isPlaying, setIsPlaying] = useState(() => here?.isPlaying() === true);
  if (here === undefined || origin === undefined) {
    return null;
  }
  if (isPlaying) {
    return (
      <>
        <hr className="cast-to__separator" />
        <button
          type="button"
          onClick={() => {
            here.stop();
            setIsPlaying(false);
          }}
        >
          <FindDevicesIcon />
          {t("cast.stopAirPlay")}
        </button>
      </>
    );
  }
  return <AirPlayStart here={here} origin={origin} onStarted={props.onStarted} />;
}

/** AirPlay pressable once the grant asked for as it shows is had; pressed, the film goes to it. */
function AirPlayStart(props: {
  readonly here: AirPlayHere;
  readonly origin: string;
  readonly onStarted?: (() => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { here, origin } = props;
  // A grant per opening (D473): one kept from an earlier opening may be revoked by its AirPlay's end.
  const [grant] = useResource(
    `airplay-grant/${here.mediaToken}`,
    async () => await makeAirPlayGrant({ api, log: noLog }, here.mediaToken, origin),
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
            props.onStarted?.();
          }
        }}
      >
        <FindDevicesIcon />
        {t("cast.airPlay")}
      </button>
    </>
  );
}
