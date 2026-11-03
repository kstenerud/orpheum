> **ABOUTME:** Principle §28: Every message may be lost and every copy of state may drift, so
> each exchange has a way to notice it and recover.

# §28 — Every message may be lost and every copy of state may drift; each exchange recovers

**Assume every message will be lost somehow, and every copy of state will fall out of step
somehow** (the owner, 2026-11-03: "We must always assume that messages will get lost somehow,
and state will get out of sync somehow. We must ensure that we have mitigations to deal with
it."). A request's channel that never opens, a connection that drops after the request left, a
server that restarts and forgets what it held in memory, a push that never arrives: each is the
ordinary case, not an incident.

The rule that makes it survivable:

> **No side may wait on another without a bound, and no side may keep a belief about shared
> state that nothing will ever correct.**

What that asks of each exchange:

- **Every wait has a bound,** and what the bound's end does is designed, not a hang: a request
  silent too long fails (rtc-request.ts's silentAtMostMs), an invitation unanswered is given up.
- **A person always has a way out** of a state waiting on someone else: a cast still being asked
  about can be stopped.
- **A failure that matters is tried again or said,** never swallowed into a state that claims
  success: Stop casting tries once more before playing the film here.
- **An answer is read before acting on it:** a receiver's Use goes into receiving only once the
  server takes it.
- **State held on two sides is compared again,** and a difference is taken as the other side's
  loss: a cast the server no longer lists is over, whatever the client believed.
- **A retry is safe,** so the first two are cheap: a request that changes state carries an
  idempotency key.

**This is [§14](14-a-cache-may-always-be-wrong.md)'s sibling.** §14 says a local copy may be
stale; this says the message that would have corrected it may never come.

**Bites when:** a screen can only be left by an answer from elsewhere, a `.catch(() => undefined)`
hides a failure the person acts on, a promise is awaited with no bound, or the answer to "what
if the server restarts now?" is "it won't".
