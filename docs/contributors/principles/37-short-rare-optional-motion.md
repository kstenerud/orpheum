> **ABOUTME:** Principle §37: Transitions are brief, nothing moves on its own, and a person who asks
> for less motion gets none.

# §37 — Motion is short, rare and optional

A hover or a focus takes 120ms (`--duration-quick`); a pop-up or a sheet takes one of the longer
duration tokens, never a length of its own. Nothing moves on its own except the status tray's slow
pulse, and that stops while a film plays: motion beside a film competes with it, and on a TV a long
transition reads as lag.

`prefers-reduced-motion` zeroes every transition, because motion is a decoration of what happens and
never the only sign that it did.

**Bites when:** an animation outlasts the duration tokens, something moves unasked, or a transition
ignores reduced motion.
