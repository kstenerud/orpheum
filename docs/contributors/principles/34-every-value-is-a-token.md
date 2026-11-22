> **ABOUTME:** Principle §34: Colours, sizes, spaces, corners, durations and layers are tokens; a
> part styles only itself, and no browser default shows through.

# §34 — Every value is a token

Every value a stylesheet uses is a custom property in `tokens.css`. A theme restyles the client by
overriding those properties, never a part's rules. A value written out in one part is one a theme
cannot reach, and the next part writes it a little differently; a page then looks assembled rather
than designed.

A part styles only its own pieces, and a browser default never shows through: a native control's own
look, or its focus style, is a part not yet built. The standard's rules 1 and 11
([ui.md](../standards/ui.md)) say what that asks.

**Bites when:** a component's stylesheet writes a colour, a recurring length or a `z-index` number,
or a native control appears in its browser's own dress.
