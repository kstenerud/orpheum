> **ABOUTME:** Telling somebody something happened — the sink this tree has never had, for two
> event sources it already has. Four operational triggers, one webhook, and a hard rule about
> what a notification may contain.

# Notifications

- **Status:** UNSPECIFIED — gap identified 2026-09-21 ([D149](../../decisions/working-notes.md)), from the source sweep in [feature-sweep-2026-09-21.md](../research/feature-sweep-2026-09-21.md).
- **Depends on:** [observability.md](observability.md) (the other consumer of the same events), [auth.md](auth.md) (who may configure a destination), [threat-model.md](threat-model.md) (a notification leaves the house)
- **See also:** [bug-reports.md](bug-reports.md) — the after-it-broke surface

## Why this exists

We have **two event sources and no sink**.
[D8](../../decisions/working-notes.md)'s change journal says exactly what changed in the
library; [D81](../../decisions/working-notes.md)'s append-only play log says exactly what
happened in a session. [observability.md](observability.md) exposes aggregates to a dashboard
and states outright that it emits *"a count and an oldest-age, never a per-item
notification"* — correctly, for a metrics endpoint. Nothing anywhere pushes a message to a
person.

The field's answer is a companion application. **Tautulli ships 22 triggers and 28 delivery
agents**; Jellyfin has 14 `NotificationType` values in its model and no delivery, pushing that
to a 251★ plugin.

## The triggers worth having are operational, not social

Tautulli's list is long and most of it is somebody watching their friends watch things. Four
are the ones an owner actually needs, and all four are already observable here:

| Trigger | Source we already have | Why it matters |
| --- | --- | --- |
| **Buffering** | [D86](../../decisions/working-notes.md)'s buffer signal, [D92](../../decisions/working-notes.md)'s failure reasons | the complaint arrives as *"it keeps stopping"* days later, or as a notification now |
| **Concurrent-stream ceiling hit** | the limit [D149](../../decisions/working-notes.md) adds to the role | otherwise a refusal is indistinguishable from a bug |
| **New device authenticated** | [D93](../../decisions/working-notes.md)'s pairing | the only security event a household owner will act on |
| **Unreachable from outside** | [D73](../../decisions/working-notes.md)'s per-component reporting, the relay | the failure nobody is present to see |

**New content added** is the one social trigger with real demand (*"Notification for added
media"*), and it is the one that needs rate discipline: the first scan of a 20,000-item
library must produce **one** message, not twenty thousand — the same first-scan hazard
[D118](../../decisions/working-notes.md) and [D133](../../decisions/working-notes.md) already
handle by batching and by counting rather than listing.

## The mechanism is one webhook

A signed HTTP POST with a typed JSON body, and nothing else in the core. Discord, Telegram,
ntfy, Gotify, Slack, MQTT and the rest are **somebody else's bridge**, which is
[D15](../../decisions/working-notes.md)'s out-of-process adapter tier and
[D20](../../decisions/working-notes.md)'s seam. Twenty-eight built-in agents is twenty-eight
vendor APIs to keep working, and Tautulli's own list is the evidence of what that costs.

## What a notification may contain

This is the part that needs deciding before anything is built, because **a notification leaves
the house**:

- [D13](../../decisions/working-notes.md) is a privacy promise and a webhook is an outbound
  connection to a third party the user chose. It is consent, not a contradiction — but the
  body must be as small as the trigger allows, and a destination is a configured secret.
- **A notification must respect what its audience may see.** *"Added: <title>"* fired at a
  household channel discloses a title a restricted role is not permitted
  ([D132](../../decisions/working-notes.md), [D134](../../decisions/working-notes.md)) — and
  [D134](../../decisions/working-notes.md)'s rule that a pending item discloses **nothing**
  applies with more force here, because a webhook body cannot be un-sent.
- **Never a credential, never a stream URL, never a path.** The same list
  [observability.md](observability.md) already refuses to expose.

## The questions nobody has answered

- **Who owns a destination — the server or the user?** A household channel is the server's; a
  "your download finished" is the user's. Probably both, which makes it an `auth.md` object.
- **Is a notification retried?** A webhook that fails at 3 a.m. and is never retried is worse
  than none, and a queue with retries is durable state.
- **Does the play log fan out per event, or on a debounce?** Ten pause/resume events in a
  minute is one human fidgeting.

## Done when

An owner learns their server is unreachable from outside without being told by a family
member; a buffering session and a refused stream are distinguishable from a bug; a first scan
produces one message; and no notification body has ever contained a title its audience was not
permitted to see.
