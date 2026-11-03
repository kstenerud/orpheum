// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { i18n } from "./i18n.ts";

import { act, type JSX } from "react";
import { beforeEach, describe, expect, it } from "vitest";

import { timeText } from "./date-time-text.ts";
import { DebugToolsContext } from "./debug-tools.tsx";
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
import { withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";
import { LiveProvider } from "./live.tsx";
import { tabStorage } from "./browser-storage.ts";
import { keepMembership } from "./together-storage.ts";
import { filmVolume, setFilmVolume } from "./film-volume.ts";
import { HostVoiceProvider } from "./together-voice.tsx";
import { TogetherDialog } from "./together-dialog.tsx";

/** Watch Together's dialog, open over the page, as the tray's sign opens it (D384 §2). */
function WatchTogether(): JSX.Element {
  return <TogetherDialog onClose={() => undefined} />;
}

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

describe("the host's Watch Together dialog (D246 §3, §5; D384 §2)", () => {
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

  it("closes Invite on a press outside it (the owner, 2026-10-29)", async () => {
    hostSession();
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
    await click(buttonWithText(mounted.container, "Invite someone"));
    const dialog = mounted.container.querySelector("dialog.watch-together__dialog");
    expect(dialog).not.toBeNull();
    // On the backdrop: the press reaches the dialog itself, at a point outside its box.
    await act(async () => {
      dialog?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: -10, clientY: -10 }));
      await Promise.resolve();
    });
    expect(mounted.container.querySelector(".watch-together__dialog")).toBeNull();
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

  it("says until when each waiting link lets its person in (D346)", async () => {
    hostSession();
    const expiresAt = Date.UTC(2026, 9, 26, 21, 40);
    const waiting = { id: 4, rights: pauseOnly, hostNote: "Grandma", expiresAt };
    const services = servedBy(
      {
        "GET /together/3": [
          200,
          { sessionId: 3, nextRights: allRights, links: [waiting], members: [karl] },
        ],
      },
      [],
      "token",
    );
    const mounted = await mount(withServices(<WatchTogether />, services));
    expect(rowStarting(mounted.container, "A link for Grandma")?.textContent).toBe(
      `A link for Grandma, not used yet, may play and pause, good until ${timeText(expiresAt, i18n.language)}`,
    );
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

  it("forgets a session the server says is over, asking for it no more, and offers a new one", async () => {
    hostSession();
    const heard: Heard[] = [];
    const services = servedBy({ "GET /together/3": refusal(404, "not_found") }, heard, "token");
    const mounted = await mount(withServices(<WatchTogether />, services));
    expect(mounted.container.querySelector("output")?.textContent).toBe("That session is over.");
    expect(buttonWithText(mounted.container, "Start a session")).toBeDefined();
    // Remembered, the session was asked for again at every background check (staging, 2026-10-28).
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
    expect(heard.filter((each) => each.methodAndPath === "GET /together/3")).toHaveLength(1);
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
    await click(
      (dialog ?? mounted.container).querySelector<HTMLButtonElement>(".modal-dialog__close button"),
    );
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
    expect(buttonWithText(mounted.container, "Start a session")).toBeUndefined();
    mounted.unmount();
  });
});

describe("what plays, in the host's Watch Together dialog (D364 §13, §15; D384 §2)", () => {
  /**
   * The session page of the host of session sessionId, whose film 4, Metropolis, plays for the group; a
   * session of its own for each test, whose details no earlier one has read, so the live connection
   * opens first.
   */
  async function sessionPage(
    heard: Heard[],
    sessionId: number,
    showsDebugTools = false,
  ): Promise<{
    readonly mounted: Awaited<ReturnType<typeof mount>>;
    readonly services: ReturnType<typeof servedBy>;
    readonly sockets: ReturnType<typeof liveAndSession>;
  }> {
    window.sessionStorage.setItem(
      "orpheum.together.host",
      JSON.stringify({ sessionId, credential: "host-credential" }),
    );
    const services = servedBy(
      {
        [`GET /together/${String(sessionId)}`]: [
          200,
          {
            sessionId,
            nextRights: allRights,
            links: [],
            members: [karl],
            fileId: 4,
            titleName: "Metropolis",
            durationMs: 600_000,
          },
        ],
        "POST /me/casts/9/end": [200, { positionMs: 0, isPlaying: false }],
      },
      heard,
      "token",
    );
    const sockets = liveAndSession();
    const mounted = await mount(
      withServices(
        <DebugToolsContext value={{ isShown: showsDebugTools, onChanged: () => undefined }}>
          <LiveProvider>
            <WatchTogether />
          </LiveProvider>
        </DebugToolsContext>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.session.accept();
    await sockets.session.say({
      type: "welcome",
      seq: 1,
      memberId: 1,
      deviceId: 7,
      isHost: true,
      group: { state: "Paused", positionMs: 61_000, rate: 1, fileId: 4 },
      roster: [],
    });
    return { mounted, services, sockets };
  }

  // The owner, 2026-10-30: "ultimately there is one playback page that everything goes through".
  it("holds the way back to the film, whose Play page holds its controls, and Stop for everyone (D384 §2)", async () => {
    const { mounted, sockets } = await sessionPage([], 31);
    const remote = sockets.session;
    expect(remote.heard[0]).toMatchObject({ credential: "host-credential", role: "panel" });
    expect(mounted.container.querySelector("video")).toBeNull();
    expect(mounted.container.textContent).toContain("Metropolis");
    expect(mounted.container.textContent).not.toContain("1:01 / 10:00");
    expect(mounted.container.querySelector('button[aria-label="Play"]')).toBeNull();
    expect(mounted.container.querySelector('a[href="/play/4"]')?.textContent).toBe(
      "Back to the film",
    );
    // The page's primary action, drawn as the title page's Play: missed as a plain link (2026-10-29).
    expect(mounted.container.querySelector('a[href="/play/4"]')?.className).toBe("title__play");
    await click(buttonWithText(mounted.container, "Stop the film for everyone"));
    expect(remote.heard.at(-1)).toMatchObject({ type: "command", command: "stop" });
    mounted.unmount();
  });

  it("marks the device leading and this one under the people, for a host who shows debug tools", async () => {
    const leading = {
      type: "roster",
      roster: [
        {
          ...karl,
          devices: [
            { id: 7, role: "panel" },
            { id: 9, role: "video", isLeader: true, leadsBecause: "putOn" },
          ],
        },
      ],
    };
    const hidden = await sessionPage([], 33);
    await hidden.sockets.session.say(leading);
    expect(hidden.mounted.container.textContent).not.toContain("▶ leading");
    hidden.mounted.unmount();
    const { mounted, sockets } = await sessionPage([], 34, true);
    await sockets.session.say(leading);
    const lines = [...mounted.container.querySelectorAll(".together-debug li, .together-debug p")];
    expect(lines.map((line) => line.textContent)).toEqual([
      "Karl: device 7 (panel) · this device",
      "Karl: device 9 (video) · ▶ leading",
      "Paused · leader device 9 (Karl) · why putOn · this device 7",
    ]);
    mounted.unmount();
  });

  it("holds Stop casting too while this device casts (D364 §11)", async () => {
    const heard: Heard[] = [];
    const { mounted, sockets } = await sessionPage(heard, 32);
    expect(buttonWithText(mounted.container, "Stop casting")).toBeUndefined();
    await sockets.live.say({
      type: "sending",
      sending: { castId: 9, receiverDeviceId: 2, receiverName: "Fire TV", isAccepted: true },
    });
    expect(mounted.container.textContent).toContain("Casting to Fire TV.");
    await click(buttonWithText(mounted.container, "Stop casting"));
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
      reason: "stopped",
    });
    mounted.unmount();
  });
});
