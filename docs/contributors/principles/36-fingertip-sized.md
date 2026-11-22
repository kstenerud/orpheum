> **ABOUTME:** Principle §36: Every control takes a press over at least 44px square, and works by
> touch, keyboard and remote as well as by a mouse.

# §36 — Fingertip-sized, for every hand and every remote

The same client runs on a phone, a touch screen, a TV and a desk, so the least precise of them sets
the size: every control's hit area is at least `size-control` (44px) square. What is drawn can be
smaller, a 20px checkbox or a 24px icon; the area that takes the press cannot.

For the same reason the mouse is not the main input but one of four. Hover only adds to what is
already there, nothing waits in a tooltip alone, and nothing is reachable only by a pointer or only
by a drag: a TV and a phone have no hover, and a remote has no pointer at all.

**Bites when:** a target is drawn and sized as one, a hover style sits outside `@media (hover:
hover)`, or a control is reached only by a pointer or a drag.
