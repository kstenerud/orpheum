// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import type { CastToReceive } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { ControlledMode, useEndsAsTheCastDoes } from "./cast-receiver.tsx";
import { askPageFullScreen, leaveFullScreenAsked } from "./film-fullscreen.ts";
import { useIsFullScreen } from "./full-screen-button.tsx";
import { IconButton } from "./icon-button.tsx";
import { Link } from "./link.tsx";
import { useLive } from "./live.tsx";
import { FullScreenIcon, LeaveFullScreenIcon } from "./player-icons.tsx";
import { useToken } from "./services.tsx";
import { FollowsTheGroupHere } from "./together-viewer.tsx";
import { useTogether } from "./use-together.ts";
import { useWarmsPlay } from "./warm-play.ts";

/**
 * A cast's receiver between videos (D364 §5): full screen, "Ready, controlled from ‹Device›", so the
 * next video the person plays starts full screen and with sound, with no press here. Its ⤢ leaves
 * full screen and goes back into it, as the player's does while a film plays, and leaving full
 * screen, by ⤢, Back or Escape, ends nothing: the device that cast ends the cast (the owner,
 * 2026-11-07, the 11-20 batch's answer 4; ControlledMode), which goes to this device's home page, as
 * every end does (D384 §1). A guest's cast,
 * handed their part in the Watch Together session they are in, follows that session's film here, just
 * another follower, as the guest's own device did (D371 §4). Without a cast waiting ready, as a page
 * loaded again before it is handed its cast again, it says so.
 */
export function CastReady(): JSX.Element {
  const { t } = useTranslation();
  const { receiving } = useLive();
  if (receiving?.fileId !== 0) {
    return (
      <section className="cast-ready-none">
        <p>{t("cast.notReceiving")}</p>
        <Link to={{ screen: "home" }}>{t("cast.toHome")}</Link>
      </section>
    );
  }
  return <ReadyFor key={receiving.castId} cast={receiving} />;
}

/** The ready page of one cast, which hears how the cast ends from its session. */
function ReadyFor(props: { readonly cast: CastToReceive }): JSX.Element {
  const { t } = useTranslation();
  const token = useToken();
  const { leaveReceiving } = useLive();
  const { cast } = props;
  useWarmsPlay(); // the next video plays here: its Play page's code is ready for it
  const { snapshot } = useTogether({
    credential: cast.membershipCredential,
    sessionToken: token,
    role: "panel", // it plays nothing; it hears the cast's end
  });
  // The cast's end goes home, however it ends (D384 §1).
  useEndsAsTheCastDoes(
    snapshot?.status === "ended" ? snapshot.endReason : undefined,
    leaveReceiving,
  );
  const { togetherCredential } = cast;
  return (
    <div className="cast-ready">
      {togetherCredential !== undefined && cast.isTogetherFollower === true ? (
        <FollowsTheGroupHere credential={togetherCredential} />
      ) : (
        <p className="cast-ready__line">{t("cast.ready", { name: cast.fromDeviceName })}</p>
      )}
      <ControlledMode fromName={cast.fromDeviceName} />
      <ReadyFullScreen />
    </div>
  );
}

/**
 * The ready page's ⤢: in full screen it leaves, as the page's own asking; out of it, it puts the
 * page back, which the next video's player keeps (askPageFullScreen, D364 §3, §5). A receiver with
 * nothing playing could leave full screen only by Escape, which ended its cast (the owner,
 * 2026-11-07).
 */
function ReadyFullScreen(): JSX.Element {
  const { t } = useTranslation();
  const isFullScreen = useIsFullScreen();
  return (
    <div className="cast-ready__full-screen">
      <IconButton
        isOverVideo
        label={isFullScreen ? t("playerControls.leaveFullScreen") : t("playerControls.fullScreen")}
        icon={isFullScreen ? <LeaveFullScreenIcon /> : <FullScreenIcon />}
        onClick={() => {
          if (isFullScreen) {
            leaveFullScreenAsked(document);
          } else {
            void askPageFullScreen(document);
          }
        }}
      />
    </div>
  );
}
