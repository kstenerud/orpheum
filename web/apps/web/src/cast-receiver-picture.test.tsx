// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastReceiver } from "./cast-receiver.tsx";
import {
  newPicturePanel,
  PicturePanelContext,
  useOriginalOfferedToARemote,
} from "./picture-panel.ts";
import { SubtitleControls, type SubtitleChoice, type SubtitleTrack } from "./subtitles.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";

/** A cast handed to this device: file 4 from 61 s, cast from the Pixel. */
const cast = {
  castId: 9,
  membershipCredential: "receiver",
  fileId: 4,
  itemName: "Metropolis",
  positionMs: 61_000,
  fromDeviceId: 1,
  fromDeviceName: "Pixel",
};

/** The receiver's welcome to its cast's session of one, nothing on yet. */
const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 9,
  memberId: 2,
  deviceId: 6,
  isHost: true,
  rights: { canPlayPause: true, canSeek: true },
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [],
};

function track(overrides: Partial<SubtitleTrack>): SubtitleTrack {
  return {
    streamId: 5,
    language: "en",
    isDefault: false,
    isForced: false,
    isHearingImpaired: false,
    assUrl: "/api/v1/bytes/media/sub/t/5.ass",
    vttUrl: "/api/v1/bytes/media/sub/t/5.vtt",
    ...overrides,
  };
}

/** The ladder's Play original, as play.tsx's LadderUnder offers it to a remote. */
function OriginalOffered(props: { readonly onChosen: () => void }): JSX.Element | null {
  useOriginalOfferedToARemote(props.onChosen);
  return null;
}

describe("what the receiver shows, worked by its remote (picture-panel.ts, D440 §5)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    document.body.replaceChildren();
  });

  // A Fire TV's own menus are hidden while it is controlled, so what it shows could only be chosen
  // after taking control back.
  it("carries out its remote's subtitle and original choices as its own presses, and tells its remote what it shows", async () => {
    const services = servedBy({}, [], "token");
    const server = fakeTogetherServer();
    const video = document.createElement("video");
    document.body.append(video);
    const chosenSubtitles = vi.fn();
    const chosenOriginal = vi.fn();
    // The page's subtitle choice, as useSubtitleChoice holds it: a choice shows its track.
    function Page(): JSX.Element {
      const [chosenTrackKey, setChosenTrackKey] = useState<string | undefined>(undefined);
      const choice: SubtitleChoice = {
        tracks: [track({}), track({ streamId: 7, language: "fr" })],
        chosenTrackKey,
        choose: (key) => {
          chosenSubtitles(key);
          setChosenTrackKey(key);
        },
        offsetMs: 0,
        shiftOffset: () => undefined,
        failure: undefined,
      };
      return (
        <PicturePanelContext value={panel}>
          <CastReceiver
            receiving={{ cast, onLeave: vi.fn() }}
            fileId={4}
            video={video}
            startPositionMs={0}
            delivery="converted"
          />
          <SubtitleControls choice={choice} />
          <OriginalOffered onChosen={chosenOriginal} />
        </PicturePanelContext>
      );
    }
    const panel = newPicturePanel();
    mounted = await mount(
      withServices(<Page />, { ...services, openTogetherSocket: server.openSocket }),
    );
    await server.accept();
    await server.say(welcome);
    const lastPicture = (): unknown =>
      (
        server.heard.filter((each) => each["type"] === "state").at(-1)?.["playback"] as
          { picture?: unknown } | undefined
      )?.picture;
    expect(lastPicture()).toEqual({
      subtitles: [
        { key: "s5", label: "English" },
        { key: "s7", label: "French" },
      ],
      canPlayOriginal: true,
    });

    await server.say({
      type: "picture",
      fromDeviceId: 5,
      picture: { kind: "subtitles", key: "s7" },
    });
    await server.say({ type: "picture", fromDeviceId: 5, picture: { kind: "original" } });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(chosenSubtitles).toHaveBeenCalledWith("s7");
    expect(chosenOriginal).toHaveBeenCalledTimes(1);
    expect(lastPicture()).toMatchObject({ subtitleKey: "s7" }); // its remote is told what it shows

    await server.say({ type: "picture", fromDeviceId: 5, picture: { kind: "subtitles" } });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(chosenSubtitles).toHaveBeenLastCalledWith(undefined);
  });
});
