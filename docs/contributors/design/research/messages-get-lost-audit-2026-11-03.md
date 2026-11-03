# The device exchanges against principles §28 (2026-11-03)

The owner asked, after the stuck cast request of 2026-11-03, for the other device exchanges to be
audited against [§28](../../principles/28-messages-get-lost.md) (every message may be lost and every
copy of state may drift): Watch Together, playback reports and the live connection. The lead's
proposal, which the owner took: a written review first, then only what gets someone stuck fixed
before the prod release. Casting itself was fixed before this review (the Cast dialog's cancel, the
core's end of an unanswered cast, Stop tried twice and said). A read-only pass over the code, each
finding confirmed by its code path; the lead checked S1 again before fixing it.

## Stuck: a person cannot get out without a reload

**S1. A hello that came late was taken as a refusal. Fixed** ("fix(core): a socket whose hello comes late is told to try again, not refused"). The core closed a socket
that said no hello within `helloWait` (10 s), or whose first read failed, with 1008, the code of a
refused sign-in, before it knew which kind of socket it was. The live connection
(`live-connection.ts`) takes 1008 as refused and never connects again: the device left every cast
list, and was handed no cast, silently, until a reload. Watch Together (`together-connection.ts`)
took it, before its welcome, as its session gone, and `together-session-end.ts` forgot the
membership, which a guest who came by a link alone cannot get back. A phone changing networks just
after its socket opened was enough. Now closed 1013 (try again later), which both clients answer by
connecting again; 1008 stays for a first message that is no hello and a refused credential.

## Lost or said wrongly; not stuck. Proposals for the owner

**S2. A host's command pressed while the page reconnects is dropped without a word.**
`TogetherConnection.command` sends only while connected and says nothing otherwise: Stop the film
(`together-plays.tsx`), the remote's buttons (`session-remote.tsx`), a change of film
(`together-host.tsx`). Pressing again after the reconnect works, so no one is stuck, but the press
looked taken. Partly covered: a player that leads sends its state again at the welcome, which places
the group, so its own pause or seek is said again; a page with no player (the remote, Stop) is not.
*Proposal:* the buttons that command a group show they wait while the connection is not up
("Reconnecting…", the buttons off), and `command` says whether it went. Holding a press until the
welcome instead could play it many seconds late, which seems worse.

**L1. The last position can be lost as a page closes.** `report.ts`'s `flush` returns while a
report is in flight and nothing flushes again when it settles; a paused player sends none; and the
report sent as the page is hidden is a plain fetch, which the browser may cancel as the tab goes.
Continue Watching then starts up to 10 s early (`reportEveryMs`), and the pause or abandon event can
be lost. *Proposal:* flush again when a report settles with more waiting, and send the hide and
`pagehide` report with `keepalive`.

**L2. A report sent again after its answer was lost is logged twice.** A failed report's events and
spans go again under a new `reportSeq`, which passes the core's check of a report seen twice
(`internal/core/playback/report.go`); its play events and spans are stored twice. The position is
safe (it moves only forward). *Proposal:* send it again under its own `reportSeq`.

**L3. A failed lookup of one's sessions is told as "the session is gone".**
`together-session-end.ts` marks the lookup done even when it failed, and no membership kept then
reads as gone: a signed-in guest is sent home, told the session is not there, while it may still
run. *Proposal:* mark it done only on an answer; on a failure, say so with a way to try again.

**L4. Ending a session has no idempotency key** (`watch-together.tsx`). With its answer lost, the
next try is answered 404 and the page shows a problem, and forgets that this tab ended it; the
host's panel still hears the end and goes home, but says it ended elsewhere. *Proposal:* take
`not_found` as ended, as Leave does (`together-leave.tsx`).

**D1. The device list is not marked stale while the live connection is down.** The lists and
`sending` are kept while disconnected, and `live.tsx` does not pass `isConnected` on, so Cast can
offer a device that has gone until the reconnect sends everything again. *Proposal:* pass
`isConnected` on, and show the list as reconnecting while it is false.

## What already holds

- **Bounded waits:** unused invitation links expire and idle sessions end
  (`internal/core/together/lifetimes.go`, `EndStale`); a cast unanswered ends (`live.UnansweredAfter`).
- **Silence both ways:** the core ends a socket silent past its limit (`readInto`, `readLive`); both
  clients drop a socket after two unanswered pings 15 s apart, and check again as the page is shown.
- **Reconnects back off,** 0.5 s to 30 s, for both connections (`together-connection.ts`).
- **Full state again on reconnect, never only changes:** the live registry sends `sending` (none
  included), the devices and any cast waiting (`internal/core/live/registry.go`); Watch Together's
  welcome carries the group, its sequence, the roster and rights (`hub.go`), and the client sends
  its own state back.
- **A session ended while a page was away,** or the core restarted: the page's reconnect is refused
  and it ends; a hosted session the core no longer lists is forgotten (`together-mine.ts`).
- **Playback reports:** an older report never moves a newer position back
  (`internal/store/queries/play.sql`); the same report twice is taken once; a failed report is kept
  for the next.
- **Leave** carries an idempotency key and takes `not_found` as left (`together-leave.tsx`).
