// SPDX-License-Identifier: MPL-2.0

import { dataOrThrow, failureText, isNearItsEnd, type Api, type components } from "@orpheum/sdk";

/**
 * A Chromecast's Up next (D471, the cells' table: a Google Cast device without a remote): near the
 * film's end it shows the next episode and that it is chosen on the phone or computer casting, with
 * nothing to press, as the device takes nothing. It shows by the rule the caster's remote shows its
 * own Up next by (the web client's cast-remote.tsx), isNearItsEnd, the film's
 * position being the session's, which the receiver leads, and only where the next episode can be
 * read now; so the two show and go together, from the one session, with no message of their own.
 */

/** What plays after a file, as the server offers it (GET /files/{id}/up-next). */
type UpNext = components["schemas"]["UpNextBody"];

/** The place of the episode, in the web client's English words for it (its episode-place.ts). */
function placeOf(upNext: UpNext): string {
  const { episode, ordering } = upNext;
  if (ordering === "aired") {
    return `S${String(episode.groupNumber)}E${String(episode.numberInGroup)}`;
  }
  if (upNext.isOnlyGroup === true) {
    return `Episode ${String(episode.numberInGroup)}`;
  }
  const group = episode.groupName ?? `Part ${String(episode.groupNumber)}`;
  return `${group} · ${String(episode.numberInGroup)}`;
}

/**
 * What the TV shows of upNext: the offer's words, as the caster's say it, and where it is chosen; none
 * where nothing plays next, or it cannot be read now (the caster offers none then either).
 */
export function upNextLines(upNext: UpNext | undefined): readonly string[] | undefined {
  if (upNext === undefined || upNext.episode.unavailable !== undefined) {
    return undefined;
  }
  return [
    `Up next: ${placeOf(upNext)} · ${upNext.episode.name ?? ""}`,
    "Choose it on your phone or computer",
  ];
}

/** The video's events after which Up next is shown or hidden again. */
const shownAgainAfter = ["timeupdate", "seeked", "ended", "durationchange", "emptied"] as const;

/** The Up next drawn over the receiver's video, for the film it plays now. */
export class UpNextCard {
  private fileId = 0;
  private lines: readonly string[] | undefined;
  private card: HTMLElement | undefined;

  constructor(
    private readonly video: HTMLVideoElement,
    private readonly api: Api,
    private readonly note: (line: string) => void,
  ) {
    for (const type of shownAgainAfter) {
      video.addEventListener(type, () => {
        this.draw();
      });
    }
  }

  /** Asks what plays after fileId, the film the receiver plays now, 0 for none, and shows it then. */
  plays(fileId: number): void {
    if (fileId === this.fileId) {
      return;
    }
    this.fileId = fileId;
    this.lines = undefined;
    this.draw();
    if (fileId === 0) {
      return;
    }
    void this.api
      .GET("/files/{id}/up-next", { params: { path: { id: fileId } } })
      .then(dataOrThrow)
      .then((answer) => {
        if (this.fileId === fileId) {
          this.lines = upNextLines(answer.upNext);
          this.draw();
        }
      })
      .catch((cause: unknown) => {
        this.note(`could not read what plays next: ${failureText(cause)}`);
      });
  }

  /** Takes the card away, the receiver no cast's now. */
  remove(): void {
    this.fileId = 0;
    this.lines = undefined;
    this.card?.remove();
    this.card = undefined;
  }

  private draw(): void {
    const { video, lines } = this;
    const durationMs = Number.isFinite(video.duration) ? video.duration * 1000 : 0;
    const isShown =
      lines !== undefined &&
      isNearItsEnd({ atMs: video.currentTime * 1000, durationMs, hasEnded: video.ended });
    if (!isShown) {
      if (this.card !== undefined) {
        this.card.hidden = true;
      }
      return;
    }
    const card = this.cardOver(video);
    if (card === undefined) {
      return;
    }
    card.replaceChildren(
      ...lines.map((line) => {
        const each = card.ownerDocument.createElement("p");
        each.textContent = line;
        return each;
      }),
    );
    card.hidden = false;
  }

  /** The card, placed after the video in its page; none for a video in no page. */
  private cardOver(video: HTMLVideoElement): HTMLElement | undefined {
    const page = video.parentElement;
    if (this.card === undefined && page !== null) {
      this.card = page.ownerDocument.createElement("div");
      this.card.className = "receiver__up-next";
      this.card.setAttribute("role", "status");
      video.after(this.card);
    }
    return this.card;
  }
}
