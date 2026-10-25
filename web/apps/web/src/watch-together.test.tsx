// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { stallAfterMs } from "@orpheum/player";
import { playedMutedEvent } from "@orpheum/sdk";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PlayView } from "./play.tsx";
import {
  buttonWithText,
  click,
  enter,
  mount,
  refusal,
  servedBy,
  submit,
  type Heard,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { tabStorage } from "./browser-storage.ts";
import { HostPlaysTogether } from "./together-host.tsx";
import { keepMembership } from "./together-storage.ts";
import { filmVolume, setFilmVolume } from "./film-volume.ts";
import { HostVoiceProvider } from "./together-voice.tsx";
import { WatchTogether } from "./watch-together.tsx";

const karl = { id: 1, name: "Karl", isHost: true };
/** A link's rights, every one (D271 §4). */
const allRights = { canPlayPause: true, canSeek: true };
/** A link's rights with moving the position withheld. */
const pauseOnly = { canPlayPause: true, canSeek: false };

beforeEach(() => {
  window.sessionStorage.clear();
});

/** This tab hosting session 3. */
function hostSession(): void {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 3, credential: "host-credential" }),
  );
}

/** The row of the people's list whose words start with words, which opens its dialog. */
function rowStarting(container: Element, words: string): Element | undefined {
  return [...container.querySelectorAll(".watch-together__people button")].find((button) =>
    button.textContent.startsWith(words),
  );
}

describe("the host's Watch Together screen (D246 §3, §5)", () => {
  it("starts a session, makes a link with a note, and shows it to pass on", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /together": [
          200,
          { sessionId: 3, membershipCredential: "host-credential", isNew: true },
        ],
        "GET /together/3": [
          [200, { sessionId: 3, nextRights: allRights, links: [], members: [karl] }],
          [
            200,
            {
              sessionId: 3,
              nextRights: allRights,
              links: [{ id: 4, rights: pauseOnly, hostNote: "Grandma" }],
              members: [karl],
            },
          ],
        ],
        "POST /together/3/links": [201, { linkId: 4, secret: "s3cret" }],
      },
      heard,
      "token",
    );
    const mounted = await mount(withServices(<WatchTogether />, services));
    await click(buttonWithText(mounted.container, "Start a session"));
    expect(window.sessionStorage.getItem("orpheum.together.host")).toContain("host-credential");
    expect(mounted.container.textContent).toContain("No one is invited yet.");
    // Inviting is a button that opens a dialog of its own (D281 §5).
    expect(mounted.container.querySelector(".watch-together__dialog")).toBeNull();
    await click(buttonWithText(mounted.container, "Invite someone"));
    await enter(mounted.container.querySelector(".watch-together__dialog input"), "Grandma");
    // The first link starts with every right; the host withholds moving the position (D271 §4).
    const [mayPause, maySeek] =
      mounted.container.querySelectorAll<HTMLInputElement>('input[type="checkbox"]');
    expect([mayPause?.checked, maySeek?.checked]).toEqual([true, true]);
    await click(maySeek);
    await submit(mounted.container.querySelector(".watch-together__dialog form"));
    expect(heard.map((each) => each.methodAndPath)).toEqual([
      "POST /together",
      "GET /together/3",
      "POST /together/3/links",
      "GET /together/3",
    ]);
    expect(heard[2]?.body).toEqual({ hostNote: "Grandma", rights: pauseOnly });
    expect(mounted.container.querySelector("output")?.textContent).toBe(
      "http://orpheum.test/together#s3cret",
    );
    expect(
      mounted.container.querySelector('img[alt="The link for Grandma, as a QR code"]'),
    ).not.toBeNull();
    expect(mounted.container.textContent).toContain("while the session lasts");
    expect(mounted.container.textContent).toContain(
      "A link for Grandma, not used yet, may play and pause",
    );
    mounted.unmount();
  });

  it("opens Invite on its who-for field, and closes the link's dialog once someone joins by it", async () => {
    hostSession();
    const services = servedBy(
      {
        "GET /together/3": [
          [200, { sessionId: 3, nextRights: allRights, links: [], members: [karl] }],
          [
            200,
            {
              sessionId: 3,
              nextRights: allRights,
              links: [
                {
                  id: 4,
                  rights: allRights,
                  hostNote: "Grandma",
                  secret: "s3cret",
                  usedByMemberId: 2,
                },
              ],
              members: [karl, { id: 2, name: "Gran", hostNote: "Grandma", isHost: false }],
            },
          ],
        ],
        "POST /together/3/links": [201, { linkId: 4, secret: "s3cret" }],
      },
      [],
      "token",
    );
    const mounted = await mount(withServices(<WatchTogether />, services));
    await click(buttonWithText(mounted.container, "Invite someone"));
    const note = mounted.container.querySelector(".watch-together__dialog input");
    expect(document.activeElement).toBe(note);
    await enter(note, "Grandma");
    await submit(mounted.container.querySelector(".watch-together__dialog form"));
    // Gran joined by it before the host closed it: the details read again say so.
    expect(mounted.container.querySelector(".watch-together__dialog")).toBeNull();
    expect(mounted.container.textContent).toContain("Gran");
    mounted.unmount();
  });

  it("lists who joined with the host's note, and removes them", async () => {
    hostSession();
    const heard: Heard[] = [];
    const gran = { id: 2, name: "Gran", hostNote: "Grandma", isHost: false };
    const services = servedBy(
      {
        "GET /together/3": [
          [
            200,
            {
              sessionId: 3,
              nextRights: allRights,
              links: [{ id: 4, rights: allRights, hostNote: "Grandma", usedByMemberId: 2 }],
              members: [karl, gran],
            },
          ],
          [200, { sessionId: 3, nextRights: allRights, links: [], members: [karl] }],
        ],
        "POST /together/3/eject/2": [204, undefined],
      },
      heard,
      "token",
    );
    const mounted = await mount(withServices(<WatchTogether />, services));
    expect(mounted.container.textContent).toContain("Gran (Grandma)");
    expect(mounted.container.textContent).not.toContain("not used yet");
    // What is done with a person is in a dialog the row opens (D281 §5).
    expect(buttonWithText(mounted.container, "Remove")).toBeUndefined();
    await click(rowStarting(mounted.container, "Gran (Grandma)"));
    await click(buttonWithText(mounted.container, "Remove"));
    // Removing someone cannot be undone: it is asked again (D278 §4).
    await click(buttonWithText(mounted.container, "Remove them"));
    expect(heard.map((each) => each.methodAndPath)).toContain("POST /together/3/eject/2");
    expect(mounted.container.textContent).not.toContain("Gran (Grandma)");
    mounted.unmount();
  });

  it("gives a guest a new link for their place, keeping them in until it is used (D311 §5, D312 §3)", async () => {
    hostSession();
    const heard: Heard[] = [];
    const gran = { id: 2, name: "Gran", hostNote: "Grandma", isHost: false };
    const services = servedBy(
      {
        "GET /together/3": [
          [200, { sessionId: 3, nextRights: allRights, links: [], members: [karl, gran] }],
          [
            200,
            {
              sessionId: 3,
              nextRights: allRights,
              links: [{ id: 5, rights: allRights, hostNote: "Grandma" }],
              members: [karl, gran],
            },
          ],
        ],
        "POST /together/3/members/2/links": [201, { linkId: 5, secret: "n3w" }],
      },
      heard,
      "token",
    );
    const mounted = await mount(withServices(<WatchTogether />, services));
    await click(rowStarting(mounted.container, "Gran (Grandma)"));
    expect(mounted.container.textContent).toContain("Gran stays in until the new link is used.");
    await click(buttonWithText(mounted.container, "New link"));
    // No one is removed: the new link takes Gran's place only once it is used.
    expect(heard.map((each) => each.methodAndPath)).toEqual([
      "GET /together/3",
      "POST /together/3/members/2/links",
      "GET /together/3",
    ]);
    expect(mounted.container.querySelector("output")?.textContent).toBe(
      "http://orpheum.test/together#n3w",
    );
    mounted.unmount();
  });

  it("shows the session had back from the server after it drew, started on another device (D267 §2)", async () => {
    const services = servedBy(
      {
        "GET /together/3": [
          200,
          { sessionId: 3, nextRights: allRights, links: [], members: [karl] },
        ],
      },
      [],
      "token",
    );
    const mounted = await mount(withServices(<WatchTogether />, services));
    expect(buttonWithText(mounted.container, "Start a session")).toBeDefined();
    await act(async () => {
      keepMembership(tabStorage(), "host", { sessionId: 3, credential: "phone-credential" });
      await Promise.resolve();
    });
    expect(mounted.container.textContent).toContain("No one is invited yet.");
    mounted.unmount();
  });

  it("shows each link again after a reload, waiting or used, to pass on (D267 §5)", async () => {
    hostSession();
    const discoveryLink =
      "https://app.example/together#k7xq2.friend-s3cret.Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
    const gran = { id: 2, name: "Gran", hostNote: "Grandma", isHost: false };
    const services = servedBy(
      {
        "GET /together/3": [
          200,
          {
            sessionId: 3,
            nextRights: allRights,
            links: [
              {
                id: 4,
                rights: allRights,
                hostNote: "Grandma",
                secret: "gran-s3cret",
                usedByMemberId: 2,
              },
              { id: 5, rights: allRights, hostNote: "a cousin", secret: "cousin-s3cret" },
              {
                id: 6,
                rights: allRights,
                hostNote: "a friend",
                secret: "friend-s3cret",
                link: discoveryLink,
              },
            ],
            members: [karl, gran],
          },
        ],
      },
      [],
      "token",
    );
    const mounted = await mount(withServices(<WatchTogether />, services));
    await click(rowStarting(mounted.container, "Gran (Grandma)"));
    await click(buttonWithText(mounted.container, "Show link"));
    expect(mounted.container.querySelector("output")?.textContent).toBe(
      "http://orpheum.test/together#gran-s3cret",
    );
    await click(rowStarting(mounted.container, "A link for a cousin"));
    await click(buttonWithText(mounted.container, "Show link"));
    expect(mounted.container.querySelector("output")?.textContent).toBe(
      "http://orpheum.test/together#cousin-s3cret",
    );
    // While a discovery has claimed the server, the link it names in the discovery's form is passed on.
    await click(rowStarting(mounted.container, "A link for a friend"));
    await click(buttonWithText(mounted.container, "Show link"));
    expect(mounted.container.querySelector("output")?.textContent).toBe(discoveryLink);
    mounted.unmount();
  });

  it("runs voice, each person's voice volume and the film's volume beside the people (D281 §4, §5)", async () => {
    hostSession();
    const gran = { id: 2, name: "Gran", hostNote: "Grandma", isHost: false };
    const services = servedBy(
      {
        "GET /together/3": [
          200,
          { sessionId: 3, nextRights: allRights, links: [], members: [karl, gran] },
        ],
      },
      [],
      "token",
    );
    const mounted = await mount(
      withServices(
        <HostVoiceProvider membership={{ sessionId: 3, credential: "host-credential" }}>
          <WatchTogether />
        </HostVoiceProvider>,
        services,
      ),
    );
    expect(mounted.container.querySelector('section[aria-label="Voice"]')).not.toBeNull();
    expect(
      mounted.container.querySelector('.together-voice__talk[aria-label="Talk"]'),
    ).not.toBeNull();
    setFilmVolume(1);
    const film = [...mounted.container.querySelectorAll("label")].find((label) =>
      label.textContent.startsWith("Film volume"),
    );
    await enter(film?.querySelector("input"), "30");
    expect(filmVolume()).toBe(0.3);
    await click(rowStarting(mounted.container, "Gran (Grandma)"));
    const dialog = mounted.container.querySelector<HTMLElement>(
      '.watch-together__dialog[aria-label="Gran"]',
    );
    expect(dialog?.textContent).toContain("Gran's voice");
    await click(buttonWithText(dialog ?? mounted.container, "Close"));
    expect(mounted.container.querySelector(".watch-together__dialog")).toBeNull();
    setFilmVolume(1);
    mounted.unmount();
  });

  it("says when a role may not host", async () => {
    const services = servedBy({ "POST /together": refusal(403, "forbidden") }, [], "token");
    const mounted = await mount(withServices(<WatchTogether />, services));
    await click(buttonWithText(mounted.container, "Start a session"));
    expect(mounted.container.textContent).toContain("Your role may not host Watch Together.");
    mounted.unmount();
  });

  it("tells a browser that cannot keep in step that it cannot host, offering no start (D250 §7)", async () => {
    const services = { ...servedBy({}, [], "token"), canFollowAGroup: () => false };
    const mounted = await mount(withServices(<WatchTogether />, services));
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "update to iOS 17.1 or later",
    );
    expect(mounted.container.querySelector("button")).toBeNull();
    mounted.unmount();
  });
});

describe("the host's player in a session (D246 §5)", () => {
  it("puts the film on for everyone from where it starts, and stops it on leaving", async () => {
    hostSession();
    const server = fakeTogetherServer();
    const services = { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket };
    const mounted = await mount(
      withServices(
        <HostPlaysTogether
          fileId={5}
          video={null}
          startPositionMs={61_000}
          cannotKeepUp={undefined}
        />,
        services,
      ),
    );
    await server.accept();
    expect(server.heard[0]).toMatchObject({
      type: "hello",
      credential: "host-credential",
      sessionToken: "token",
    });
    await server.say({
      type: "welcome",
      seq: 1,
      memberId: 1,
      isHost: true,
      group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
      roster: [{ ...karl, devices: [{ id: 9, role: "playback" }] }],
    });
    expect(server.heard).toContainEqual({
      v: 2,
      type: "command",
      command: "changeItem",
      fileId: 5,
      positionMs: 61_000,
      basedOnSeq: 1,
    });
    expect(mounted.container.textContent).toContain("Watch Together is on");
    // The film's volume beside each person's voice, on the host's player too (the owner, 2026-10-12).
    expect(mounted.container.querySelector(".film-volume input")).not.toBeNull();
    mounted.unmount();
    expect(server.heard.at(-1)).toMatchObject({ type: "command", command: "stop" });
  });

  it("offers the host a tap for sound when the browser plays the film muted, as after a reload", async () => {
    hostSession();
    const server = fakeTogetherServer();
    const services = { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket };
    const video = document.createElement("video");
    const mounted = await mount(
      withServices(
        <HostPlaysTogether
          fileId={5}
          video={video}
          startPositionMs={61_000}
          cannotKeepUp={undefined}
        />,
        services,
      ),
    );
    await server.accept();
    expect(buttonWithText(mounted.container, "Tap for sound")).toBeUndefined();
    video.muted = true;
    await act(async () => {
      video.dispatchEvent(new Event(playedMutedEvent));
      await Promise.resolve();
    });
    await click(buttonWithText(mounted.container, "Tap for sound"));
    expect(video.muted).toBe(false);
    expect(buttonWithText(mounted.container, "Tap for sound")).toBeUndefined();
    mounted.unmount();
  });

  it("follows the player's new video when it moves to a lower quality, stopping nothing", async () => {
    hostSession();
    const server = fakeTogetherServer();
    const services = { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket };
    let showVideo: (video: HTMLVideoElement) => void = () => undefined;
    function Moving(): JSX.Element {
      const [video, setVideo] = useState(() => document.createElement("video"));
      showVideo = setVideo;
      return (
        <HostPlaysTogether
          fileId={5}
          video={video}
          startPositionMs={61_000}
          cannotKeepUp={undefined}
        />
      );
    }
    const mounted = await mount(withServices(<Moving />, services));
    await server.accept();
    await server.say({
      type: "welcome",
      seq: 1,
      memberId: 1,
      isHost: true,
      group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
      roster: [{ ...karl, devices: [{ id: 9, role: "playback" }] }],
    });
    await act(async () => {
      showVideo(document.createElement("video"));
      await Promise.resolve();
    });
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Playing", positionMs: 61_000, rate: 1, fileId: 5 },
    });
    const commands = server.heard.filter((message) => message["type"] === "command");
    expect(commands.map((command) => command["command"])).toEqual(["changeItem"]);
    expect(server.openedCount()).toBe(1);
    mounted.unmount();
  });

  it("keeps the session through the play page's move to a lower quality, stopping nothing", async () => {
    window.localStorage.setItem("orpheum.audioOutput", "speakers");
    hostSession();
    const server = fakeTogetherServer();
    const offer = {
      url: "/api/v1/bytes/media/chunks/t/master.m3u8",
      startAtBps: 1_242_000,
      qualityKbps: 700,
      height: 432,
      isBelowTheLowest: false,
      soundCarriedGainDb: 0,
      isReady: true,
      filmPictureKbps: 2_400,
    };
    const services = {
      ...servedBy({ "POST /playback/t/report": [200, { lowerQuality: offer }] }, [], "token"),
      openTogetherSocket: server.openSocket,
    };
    const mounted = await mount(
      withServices(
        <PlayView fileId={5} decision={directDecision()} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    await server.accept();
    await server.say({
      type: "welcome",
      seq: 1,
      memberId: 1,
      isHost: true,
      group: { state: "Playing", positionMs: 0, rate: 1, fileId: 5 },
      roster: [{ ...karl, devices: [{ id: 9, role: "playback" }] }],
    });
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    // Playing, it runs out of data, and the report's answer moves it to the lower quality.
    Object.defineProperty(video, "paused", { value: false });
    await act(async () => {
      // The stall's wait on fake time, not the real clock's (rule 16; the test audit, 2026-10-08).
      vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
      try {
        video.dispatchEvent(new Event("playing"));
        video.dispatchEvent(new Event("waiting"));
        await vi.advanceTimersByTimeAsync(stallAfterMs + 50);
      } finally {
        vi.useRealTimers();
      }
    });
    expect(mounted.container.textContent).toContain("moves to a lower quality");
    expect(server.heard.filter((message) => message["type"] === "command")).toEqual([]);
    expect(server.openedCount()).toBe(1);
    mounted.unmount();
  });

  it("is what the play page does while this tab hosts a session, and nothing without one", async () => {
    const server = fakeTogetherServer();
    const services = {
      ...servedBy({}, [], "token"),
      openTogetherSocket: server.openSocket,
    };
    const view = (
      <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />
    );
    const alone = await mount(withServices(view, services));
    expect(server.openedCount()).toBe(0);
    alone.unmount();
    hostSession();
    const hosting = await mount(withServices(view, services));
    await act(async () => {
      await Promise.resolve();
    });
    expect(server.openedCount()).toBe(1);
    hosting.unmount();
  });
});
