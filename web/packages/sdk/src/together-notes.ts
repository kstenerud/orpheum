// SPDX-License-Identifier: MPL-2.0

/**
 * What a device says of its player: its state, which the group waits on, and the notes the server
 * logs. Kept apart from the connection that sends them, so the connection fits in one read.
 */

import type { VoiceCallStats } from "./together-voice-stats.ts";

/** Where this device's player is, as it tells the group. */
export interface PlayerState {
  readonly positionMs: number;
  readonly isPlaying: boolean;
  /** Not ready to play along: loading, seeking, or not yet at the group's position. */
  readonly isBuffering: boolean;
  /** How late its player has learned it starts, in ms; absent until it has started once (D282). */
  readonly startLagMs?: number;
  /**
   * How much film loaded ahead it is ready on now, and how many times faster than it plays its last
   * chunk of picture loaded: which ready rule applies (D283 §1).
   */
  readonly readyOnMs?: number;
  readonly loadsFaster?: number;
}

/**
 * One thing a device's player did, which the server logs to say why a film jumped or froze (the
 * owner's test, 2026-10-06, where nothing said what a viewer's iPhone did between its jumps):
 * - nudged: played at rate for a moment, driftMs behind the group (ahead when negative);
 * - sought: moved itself from positionMs to toMs, why: "catchUp" (jumping ahead of a group it fell
 *   behind), "hold" (to where a stopped group is), "skip" (a small seek of a player that corrects
 *   by seeks alone, D268 §2) or "runUp" (back to run up to where the group starts, D282);
 * - loadHeld: a ready player held its loading while its group waits ("held"), or went on ("goesOn")
 *   (D283 §2);
 * - correctsBySeeks: its rate's returns to the group's cost it returnCostMs, more than the drift
 *   left alone, so it corrects by seeks alone from now on (D268 §2);
 * - chunkLoaded: loaded one chunk of track ("video" or "audio") in loadMs, at loadedKbps, of a quality
 *   of qualityKbps;
 * - qualitySwitched: began playing the quality of qualityKbps, its link estimated at linkEstimateKbps
 *   once a chunk measured it;
 * - reconnected: connected again after offlineMs, why: "closed" (its socket closed) or "silent"
 *   (nothing was heard on it);
 * - voiceStats: a voice call with partnerDeviceId, as its receiver measured it (D262 §1);
 * - voiceCall: a voice call with partnerDeviceId took a step, why: VoiceCallStep, so a call that
 *   never connects says how far it got (a browser tier's run, 2026-10-07, where nothing did);
 * - voiceOff: voice turned off, why: the person left ("left") or a call could not connect
 *   ("unreachable");
 * - voiceTalk: the person's microphone opened ("talking"), closed ("stopped"), muted or unmuted
 *   (D281 §2, §3);
 * - ponged: a pong of roundTripMs said where the group is, aheadOfExpectedMs ahead of where the device
 *   expected it (behind when negative), and placed the group anew ("placed") or not ("kept"): the
 *   residuals of a placing kept for minutes are the two clocks' skew (D264, D268 §1);
 * - ownMarkExpired: a seek or pause the follower made fired no event within ownMarkLastsMs, and its
 *   mark was given up (D268 §4);
 * - groupFollowed: the player moved as the group's change seq asks, heldMs after the change reached
 *   it, why what held it that long (GroupHeldBy): a cast's receiver played on seven seconds after
 *   its remote paused it, and nothing said where those seconds went (the owner, 2026-10-28);
 * - playedMuted: the browser would not start the film with sound at positionMs, for want of a recent
 *   tap, and it plays muted (playedMutedEvent), page whether the page had been pressed
 *   (pageActivation);
 * - unmuted: the person's tap turned the sound on at positionMs (soundOnAtTap), by how they pressed,
 *   the sound's state then, and again "after", soundToldAgainAfterMs later;
 * - playedWithSound: a cast's receiver's film started with its sound on at positionMs, page the
 *   page's activation then (D358 §2);
 * - fullScreen: the player's stage was put full screen ("entered") or the browser refused
 *   ("refused"), by the press that asked, absent as the player opened; or the browser left it
 *   unasked ("left"), a Fire TV's Back or Escape, which takes a cast's control back (D338 §6);
 * - diagnosis: what the player holds at positionMs as it moved itself over a gap ("movedItself"),
 *   stalled ("stalled") or buffered a playlist's first chunk ("bufferedFirst"): its readyState, its
 *   picture's and sound's buffered ranges, its last error, how hls.js placed its first chunks and
 *   whether its media source lets it load.
 */
export type PlayerNote =
  | {
      readonly kind: "nudged";
      readonly positionMs: number;
      readonly driftMs: number;
      readonly rate: number;
      /** What the player learned its rate going back to the group's costs it (driftCorrection). */
      readonly returnCostMs: number;
    }
  | {
      readonly kind: "sought";
      readonly positionMs: number;
      readonly toMs: number;
      readonly why: "catchUp" | "hold" | "skip" | "runUp";
    }
  | {
      readonly kind: "chunkLoaded";
      readonly track: "video" | "audio";
      readonly loadMs: number;
      readonly loadedKbps: number;
      readonly qualityKbps: number;
    }
  | {
      readonly kind: "qualitySwitched";
      readonly qualityKbps: number;
      readonly linkEstimateKbps: number | undefined;
    }
  | { readonly kind: "reconnected"; readonly offlineMs: number; readonly why: "closed" | "silent" }
  | {
      readonly kind: "voiceStats";
      readonly voice: VoiceCallStats & {
        readonly partnerDeviceId: number;
        /** What the Talk button's ring reads of the voice going out now, from 0 to 1. */
        readonly ringLevel?: number;
        /** The volume this person set for the partner's voice, 0 to partnerGainMost (2026-10-12). */
        readonly partnerGain?: number;
      };
    }
  | { readonly kind: "voiceCall"; readonly partnerDeviceId: number; readonly why: VoiceCallStep }
  | { readonly kind: "voiceOff"; readonly why: "left" | "unreachable" }
  | { readonly kind: "voiceTalk"; readonly why: "talking" | "stopped" | "muted" | "unmuted" }
  | {
      readonly kind: "ponged";
      readonly roundTripMs: number;
      readonly aheadOfExpectedMs: number;
      readonly why: "placed" | "kept";
    }
  | {
      readonly kind: "playedMuted";
      readonly positionMs: number;
      /** The page's activation then (pageActivation), where the browser says it. */
      readonly page?: string;
    }
  | {
      readonly kind: "playedWithSound";
      readonly positionMs: number;
      /** The page's activation then (pageActivation), where the browser says it. */
      readonly page?: string;
    }
  | {
      readonly kind: "fullScreen";
      readonly why: "entered" | "refused" | "left";
      /** The press that asked; absent as the player opened, and for "left". */
      readonly by?: Press;
    }
  | { readonly kind: "ownMarkExpired"; readonly why: "seek" | "pause" }
  | {
      readonly kind: "groupFollowed";
      /** The change of the group the player moved for, by its number (TogetherSnapshot's groupSeq). */
      readonly seq: number;
      /** How long after the change reached it the player moved, in ms. */
      readonly heldMs: number;
      readonly why: GroupHeldBy;
    }
  | { readonly kind: "correctsBySeeks"; readonly returnCostMs: number }
  | { readonly kind: "loadHeld"; readonly why: "held" | "goesOn" }
  | {
      readonly kind: "leaderCommand";
      readonly why: "pause" | "seek" | "play";
      /** The player's last events, as "<event>[(own)]@<monotonic ms>", oldest first. */
      readonly recentEvents: string;
    }
  | {
      readonly kind: "diagnosis";
      readonly why: "movedItself" | "stalled" | "bufferedFirst";
      readonly positionMs: number;
      readonly readyState: number;
      readonly videoBuffered: string;
      readonly audioBuffered: string;
      readonly lastError: string;
      /** How hls.js placed its first chunks (diagnosisOf). */
      readonly mapping: string;
      /** Whether an iPhone's media source lets it load: "streaming", "stopped" or "". */
      readonly mediaSourceState: string;
    }
  | {
      readonly kind: "unmuted";
      readonly positionMs: number;
      /** At the tap, absent; "after", soundToldAgainAfterMs later. */
      readonly why?: "after";
      /** How the person pressed, where the page says. */
      readonly by?: Press;
      /** The sound then (soundStateOf). */
      readonly soundState?: string;
    }
  | {
      /** A guest's page asking for a tap to continue, and how the browser came to show it. */
      readonly kind: "continueAsked";
      readonly why: "navigate" | "reload" | "back_forward" | "prerender";
      /** How long since the page loaded, and whether it joined itself. */
      readonly page: string;
    };

/**
 * What held a player from following its group's change as it arrived: nothing ("atOnce"), its page
 * hidden on a handheld, whose film waits there ("hidden"), its own command awaiting the group's
 * answer ("awaitingAnswer"), or its film not yet loaded far enough to be moved ("loading").
 */
export type GroupHeldBy = "atOnce" | "hidden" | "awaitingAnswer" | "loading";

/**
 * A step a voice call takes: it sent its offer, sent its answer to one, took the answer to its
 * own, connected or failed; or a signal came from a device it has no call with, and was dropped.
 */
export type VoiceCallStep =
  "offered" | "answered" | "accepted" | "connected" | "failed" | "signalDropped";

/** How a person pressed: a touch's tap, a mouse's click, or a key (a remote's OK among them). */
export type Press = "tap" | "click" | "key";
