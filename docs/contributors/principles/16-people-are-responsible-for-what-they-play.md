> **ABOUTME:** Principle §16: People are responsible for what they play; we never supply, locate,
> unlock or promote it.

# §16 — People are responsible for what they play; we never supply, locate, unlock or promote it

**We build a player for media people have the right to use, and we do not police them.** No
filtering, no fingerprinting, no guessing from filenames, no phoning home. That is the owner's
position, and it is also the legally stronger one: what makes a tool's author liable is **what the
author says and bundles**, not what users do unprompted — and monitoring would create the very
knowledge that liability rests on ([D187](../decisions/working-notes.md),
[research/legal.md](../design/research/legal.md)).

So every feature, integration, screenshot, example and sentence of documentation passes one test:

> **Does this supply content, locate it, unlock it, or encourage getting it from somewhere the
> person has no right to?** If so, we do not build it, bundle it, link it or say it.

Canada's s.27(2.4) turns that into a checklist worth running on anything borderline: how is it
**marketed**; what do we **know** of its use; does it have **substantial lawful uses**; could we
**limit** misuse by design; do we **benefit** from misuse; would it be **viable** without it.

**What this is not.** It is not a reason to refuse generic, dual-use compatibility — a webhook
from any tool that writes files, a player that plays any codec. The line is **preconfigured
sources, circumvention and promotion**, not interoperability.

**Bites when:** a feature request says "it would be convenient if it could also fetch…", an example
uses a real film because it was to hand, or a README sentence explains where people "usually" get
their files.
