> **ABOUTME:** How desktop toolkits and the web each answered reuse, layout and styling, what the
> web has re-learned and what it still lacks, and what that means for the web client's UI
> architecture. Input to the UI principles document D245 asks for; read it before writing them.

# UI Architecture: Reuse, Layout and Styling, Then and Now

- **Date:** 2026-10-05
- **Requested by:** the owner, in [D245](../../decisions/working-notes.md) §3: *"It used to be
  that you'd make a number of common components and then re-use them, with the UI subsystem
  making sure layout flowed consistently, so that you didn't have to manually fiddle with one
  part of the UI just to make another unrelated part look right. And then that was all thrown
  out as JS and HTML became popular, and we lost over a decade re-learning those lessons (and
  I'm not sure if we've re-learned them all yet). So I want a proper research of not just what
  the web ecosystem is doing, but also contrasted with what actual best practices have been in
  the past for maintainability and ease-of-development, and what lessons we can pick up wherever
  the current 'best practices' still fall short."*
- **Sources:** toolkit documentation (Oracle, Qt, Microsoft, Apple, Embarcadero, GNOME, Google,
  Flutter), MDN, the W3C, the `web-platform-dx/web-features` data for Baseline dates, and the
  essays and design systems cited in place. Every page was fetched on 2026-10-05 and is cached
  under `/yoloai/cache/http/ui-architecture/` (raw HTML, a `.txt` extract and a `.url` file per
  page; Baseline data under `web-features/`; npm metadata in `npm-view-2026-10-05.txt`).
- **Scope:** the architecture half of D245's research. What a media server's interface should
  look like, and what competitors do, are other parts of the same research.

**How to read the claims.** *Documented* means the cited source says it, usually quoted.
*Measured* means counted in this repository on 2026-10-05. *Opinion* marks a judgement, the
source's or this document's. History, not specification: the principles document will decide.

## 1. The question, made precise

The owner's sentence names three properties a UI system used to give, and asks whether the web
gives them now:

1. **Reuse.** A small set of common components is written once and used everywhere.
2. **Layout that flows.** The system, not the author, works out where things go when content,
   window size, font or language changes.
3. **Locality.** Changing one part does not need fiddling with another unrelated part.

D245 §5 adds a fourth, specific to this project: the design will be iterated, so **a change to
one screen's look must not need changes elsewhere**, and §2 says **one skin only**.

The question for this document is therefore which mechanisms delivered those properties in the
toolkits, which of them the web now has, which it still lacks, and what the web client should
adopt so that it has all four.

## 2. What the toolkits did

### 2.1 Separation of model and presentation (Smalltalk MVC)

MVC came out of Trygve Reenskaug's work in Xerox PARC's Learning Research Group, *"from the
summer og 1978 to the summer of 1979"* ([Reenskaug's MVC
pages](https://folk.universitetetioslo.no/trygver/themes/mvc/mvc-index.html), documented). Its
lasting contribution is the split between what is shown and how it is shown. It says little
about layout. The web client already has this split, more strictly than most: the SDK holds no
React ([web-client.md](../plans/web-client.md)), and screens are pure views of their data
(`web/apps/web/README.md`).

### 2.2 Layout managers: the parent places the child

Every retained-mode toolkit since the mid-1990s has the same core protocol, under different
names. **A child reports how big it would like to be and how willing it is to stretch; the
parent decides where the child goes and how big it ends up.** The child never positions itself
and never reserves space outside itself.

- **Java Swing** ([Using Layout
  Managers](https://docs.oracle.com/javase/tutorial/uiswing/layout/using.html)): every container
  has a layout manager (`BorderLayout`, `BoxLayout`, `FlowLayout`, `GridLayout`,
  `GridBagLayout`, `GroupLayout`), and absolute positioning is the documented exception
  ("Doing Without a Layout Manager"). The failure was the general-purpose managers. Oracle's own
  tutorial says writing layout code by hand *"can be challenging"* and that `GroupLayout` *"was
  developed for GUI builders such as Matisse"*, needing *"each component … to be defined twice in
  the layout"*, once per axis
  ([GroupLayout](https://docs.oracle.com/javase/tutorial/uiswing/layout/group.html),
  documented). Third parties filled the gap ([MigLayout](https://www.miglayout.com/)). *Opinion:*
  a few simple managers composed by nesting held up; the one manager that tried to do
  everything became something only tools could write.
- **Qt** ([Layout Management](https://doc.qt.io/qt-6/layout.html), documented): layouts
  *"automatically position and resize widgets when the amount of space available for them
  changes, ensuring that they are consistently arranged"*. Each widget declares a `sizeHint()` and
  a [`QSizePolicy`](https://doc.qt.io/qt-6/qsizepolicy.html) per axis (`Fixed`, `Minimum`,
  `Maximum`, `Preferred`, `Expanding`, …) plus stretch factors. Layouts reflow when the *"font
  size, text or other contents of child widgets"* change, and `QHBoxLayout` runs *"right to left
  for right-to-left languages"* without the author doing anything.
- **WPF** ([Layout](https://learn.microsoft.com/en-us/dotnet/desktop/wpf/advanced/layout),
  documented): *"a recursive system"* of two passes, *"a measure pass and an arrange pass"*;
  each `Panel` implements `MeasureOverride` and `ArrangeOverride`. Microsoft's performance page
  warns that the layout pass is *"mathematically-intensive"* and that a changed child *"has the
  potential to trigger a new pass"* ([Optimizing Performance: Layout and
  Design](https://learn.microsoft.com/en-us/dotnet/desktop/wpf/advanced/optimizing-performance-layout-and-design)).
- **Flutter** ([Understanding
  constraints](https://docs.flutter.dev/ui/layout/constraints), documented) states the whole
  protocol in one line: *"Constraints go down. Sizes go up. Parent sets position."*
- **Jetpack Compose** ([Layout
  basics](https://developer.android.com/develop/ui/compose/layouts/basics), documented):
  *"the UI tree is laid out in a single pass"*, with `Row`, `Column` and `Box` as the basic
  containers, so *"you can nest as deeply as you want"* without repeated measurement.
- **SwiftUI** ([Layout
  fundamentals](https://developer.apple.com/documentation/swiftui/layout-fundamentals),
  documented): *"Stacks and grids update and adjust the positions of the subviews they contain
  in response to changes in content or interface dimensions"*, and *"You can nest layout
  containers inside other layout containers to any depth"*.
- **GTK** ([GtkBox](https://docs.gtk.org/gtk4/class.Box.html)) has the same boxes, and its CSS
  styles them (§2.4).

**Two weaker models show what the protocol is for.**

- **Delphi VCL** placed controls at absolute positions from the form designer, with `Anchors`
  defaulting to `[akLeft, akTop]` and `Align` for docking. Anchors *"is enforced only when the
  parent is resized"*, and only `Align` *"allows controls to adjust to changes in the size of
  other aligned sibling controls"*
  ([TControl.Anchors](https://docwiki.embarcadero.com/Libraries/Sydney/en/Vcl.Controls.TControl.Anchors),
  documented). A control that grows does not push its neighbours: the "fiddle with another part"
  problem, built in.
- **Apple's autoresizing masks** (springs and struts) *"adapt only to external changes"*, not to
  a view's own content changing ([Auto Layout
  Guide](https://developer.apple.com/library/archive/documentation/UserExperience/Conceptual/AutolayoutPG/index.html),
  documented). Apple calls them *"just an iterative improvement on programmatic layouts"*.

**And spacing was font-relative long before the web had `rem`.** Windows' layout guidelines
give every spacing in dialog units, *"device independent metrics such as dialog units (DLUs)"*,
which scale with the dialog's font: *"If controls aren't touching, have at least 3 DLUs"*, and
*"dependent controls are indented 12 DLUs"* ([Windows UX guidelines:
Layout](https://learn.microsoft.com/en-us/windows/win32/uxguide/vis-layout), documented). The
guideline's goal is the owner's: *"The layout appears as intended regardless of the font
typeface or size, dots per inch (dpi), display, or graphic adaptor."* The same page is a
**spacing system**: a short table of named distances, used everywhere.

### 2.3 Constraint layout, and the retreat to stacks

Apple went further than boxes. Auto Layout (OS X Lion, 2011) solves linear constraints with the
Cassowary algorithm ([UW Cassowary](https://constraints.cs.washington.edu/cassowary/),
documented: *"The Cassowary constraint solver is now used as the layout engine in the Macintosh
Lion OS"*). It handles *"both internal and external changes"* ([Auto Layout
Guide](https://developer.apple.com/library/archive/documentation/UserExperience/Conceptual/AutolayoutPG/index.html)).

Then Apple added stack views on top of it, and its own guide steers authors to them: stack
views' *"attributes allow a high degree of control"*, and *"You can augment these settings with
additional, custom constraints; however, this increases the layout's complexity"* ([Stack
Views](https://developer.apple.com/library/archive/documentation/UserExperience/Conceptual/AutolayoutPG/LayoutUsingStackViews.html),
documented). SwiftUI (2019) and Compose made stacks, rows, columns and grids the default and
left general constraints as a specialist tool (`ConstraintLayout`).

The web tried constraints too. *Constraint Cascading Style Sheets* (Badros, Borning, Marriott,
Stuckey, UIST 1999; listed on [Constraints and the
Web](https://constraints.cs.washington.edu/web/)) and later GSS, which *"replaces the browser's
layout engine with one that harnesses the Cassowary Constraint Solver"*
([GSS](http://gss.github.io/),
documented). Neither became a standard; CSS went to flexbox and grid instead.

*Opinion:* the history does not say the web is missing constraint layout. It says the platforms
that had constraints found a few composable containers did the everyday work better. The web
now has those containers (§3.3).

### 2.4 Theming, separated from structure

Each toolkit put the look behind a seam. The seams differ in width, and the width predicted
how well they aged.

- **Swing's pluggable look and feel** splits every component into *"a JComponent subclass and a
  corresponding ComponentUI subclass"*, the UI delegate, and each look and feel supplies the
  delegates ([How to Set the Look and
  Feel](https://docs.oracle.com/javase/tutorial/uiswing/lookandfeel/plaf.html),
  documented). Behaviour and structure stay in the component; drawing is swapped wholesale.
- **Qt** draws every widget through a `QStyle`, which *"encapsulate[s] the look and feel of a
  GUI"* ([Styles and Style Aware Widgets](https://doc.qt.io/qt-6/style-reference.html)). Qt
  then added CSS-like style sheets, and its 6.12 documentation now says they *"are a tool for
  prototyping and for small, local adjustments; for the production look of an application,
  implement a QStyle"* ([Qt Style Sheets](https://doc.qt.io/qt-6/stylesheet.html), documented).
  The reasons it gives are the web's problems, stated by a toolkit vendor ([Styling Approaches
  for Qt Widgets](https://doc.qt.io/qt-6/qtwidgets-styling-approaches.html), documented):
  - *"Selectors couple the design to implementation details. Rules match class names, object
    names, and property values. Renaming an object or replacing a widget class silently stops
    the rule from matching."*
  - *"A partial rule discards the native look … a small adjustment grows into a complete
    description of the look."*
  - *"Style sheets win over programmatic settings … so the styling of a widget is no longer
    visible in one place."*
  - *"Don't hard-code colors … Use `palette(window)`, `palette(text)`"*; *"Don't size
    everything in pixels … Use em and ex lengths"*.
  - And the narrow seam it recommends instead: *"Draw with the colors from the QPalette …
    The same style then produces a light and a dark variant of the design from two
    palettes."*
- **WPF** controls are "lookless": a control's appearance is its `ControlTemplate`, shared
  values live in resource dictionaries (`StaticResource`, `DynamicResource`), and styles
  inherit with `BasedOn` ([Styles and
  templates](https://learn.microsoft.com/en-us/dotnet/desktop/wpf/controls/styles-templates-overview),
  [XAML resources](https://learn.microsoft.com/en-us/dotnet/desktop/wpf/systems/xaml-resources-overview),
  documented). The cost is all-or-nothing replacement: *"There's no way to replace only part of
  the visual tree of a control."*
- **VCL Styles** describe *"every part and state of a control"* as bitmaps ([VCL Styles
  Overview](https://docwiki.embarcadero.com/RADStudio/Sydney/en/VCL_Styles_Overview),
  documented), and list controls the style does not reach. A style that paints over native
  controls leaves gaps where a control paints itself.
- **GTK 3 and 4** style widgets with CSS ([CSS in
  GTK](https://docs.gtk.org/gtk4/css-overview.html)). GNOME's designers argue that this was
  never a theming API: *"There are no themes, just CSS"*, the platform stylesheet is called
  Adwaita, *"'the only one' in Sanskrit"*, and *"Since CSS is a huge API surface, which is used
  by both app developers and 'theme' developers, it's very easy for the two to conflict"*
  (Tobias Bernard, [Restyling apps at
  scale](https://blogs.gnome.org/tbernard/2018/10/15/restyling-apps-at-scale/), 2018,
  opinion with examples; the open letter [Please don't theme our apps](https://stopthemingmy.app/)
  followed). He also rejects the easy fix: *"'If only app developers followed best practices,
  used CSS variables, and derived their colors from theme colors, everything would be fine' …
  this doesn't come close to solving the entire problem."*
- **Flutter** and **Compose** theme through a typed object passed down the tree: `ThemeData`
  ([Flutter themes](https://docs.flutter.dev/cookbook/design/themes)), and `MaterialTheme`'s
  colour scheme, typography and shapes, or a custom design system's values provided through a
  `CompositionLocal` ([Custom design systems in
  Compose](https://developer.android.com/develop/ui/compose/designsystems/custom),
  documented).

*Opinion, drawn from the above:* **narrow, typed seams lasted** (a palette of named roles, a set
of metrics, a theme object); **wide seams failed** (selectors that can reach any widget). This is
the same finding [D138](../../decisions/working-notes.md) §4 made about Jellyfin's CSS themes,
which break on every release because they target an unversioned DOM.

### 2.5 What went wrong, by toolkit

| Toolkit | What made layout flow | How the look was separated | What went wrong |
| --- | --- | --- | --- |
| Swing | Layout managers per container | UI delegates (PLAF) | `GridBagLayout`/`GroupLayout` too hard by hand; non-native look |
| Qt | Layouts, size hints and policies, stretch | `QStyle`, palettes | Style sheets: per-widget cost, selectors coupled to implementation (Qt's own words) |
| WPF | Measure and arrange panels | Lookless controls, templates, resources | Layout pass cost; a template is replaced whole |
| Delphi VCL | `Align` only; positions absolute | VCL Styles bitmaps | Neighbours do not reflow; styles miss some controls |
| GTK | Boxes | CSS | No theme API, so themes and apps conflict |
| Cocoa | Auto Layout constraints | System appearance | Constraint complexity; stack views added on top |
| Flutter, Compose, SwiftUI | Stacks/rows/columns, one pass | Typed theme objects | (Too recent to judge; *opinion*) |

## 3. The web's path

### 3.1 Timeline

Dates are from the cited source; "Baseline" dates are when the feature worked in every major
browser engine (`web-features` data, documented).

| When | What |
| --- | --- |
| 1996 | CSS1 a Recommendation, December ([W3C CSS history](https://www.w3.org/Style/CSS20/history.html)). Spacer GIFs published as a layout trick ([Spacer GIF](https://en.wikipedia.org/wiki/Spacer_GIF)); tables carry page layout |
| 1998 | CSS2 a Recommendation, May |
| 2006 | "In Search of the Holy Grail": a three-column layout from floats and negative margins ([A List Apart](https://alistapart.com/article/holygrail/)) |
| 2011 | Bootstrap released, August; built *"to encourage consistency across internal tools"* ([Bootstrap](https://en.wikipedia.org/wiki/Bootstrap_(front-end_framework))) |
| 2014 | Lobotomized owl (`* + *`): space set by context, not element ([A List Apart](https://alistapart.com/article/axiomatic-css-and-lobotomized-owls/), October). CSS-in-JS talk, November ([vjeux](https://speakerdeck.com/vjeux/react-css-in-js)) |
| 2015 | Flexbox Baseline, September |
| 2017 | Custom properties Baseline, April. Grid Baseline, October. Utility-first essay, August ([Wathan](https://adamwathan.me/css-utility-classes-and-separation-of-concerns/)) |
| 2021 | `gap` in flexbox Baseline, April. Logical properties and `aspect-ratio`, September |
| 2022 | Cascade layers, `:focus-visible` and `<dialog>` Baseline, March. "Why we're breaking up with CSS-in-JS", October |
| 2023 | Container size queries, February. `inert`, April. Subgrid, September. `:has()`, December |
| 2025 | Popover Baseline, January. styled-components in maintenance mode, March. Design Tokens format 2025.10, October |
| 2026 | `@scope` Baseline, March. `focusgroup` origin trial, March. The WICG spatial-navigation repository archived, March. Parts of anchor positioning Baseline, the whole not yet |

*Opinion:* the owner's "over a decade" fits the record. From CSS2 (1998) to flexbox in every
browser (2015) is seventeen years in which web layout had no general container that placed
its children; to `gap` in flexbox (2021), the parent could not even own the space between
children without margins.

### 3.2 Methodologies: conventions standing in for a missing mechanism

- **OOCSS** (Nicole Sullivan): *"Separate structure and skin"* and *"Separate container and
  content"* ([OOCSS wiki](https://github.com/stubbornella/oocss/wiki), documented). The first is
  Swing's delegate split; the second is the layout-manager rule that a thing does not depend on
  where it is placed.
- **SMACSS** sorts rules into *"Base, Layout, Module, State, Theme"* ([SMACSS](http://smacss.com/)),
  a layering that `@layer` (§3.4) now makes real.
- **BEM** prefixes every class with its block's name, a namespace by convention. This project
  uses it: *"Class names are prefixed with the component's file name"*, review-only
  ([typescript.md](../../standards/typescript.md), Styling).
- **CUBE CSS** names a *composition* layer whose job is *"to create flexible,
  component-agnostic layout systems"*, and which should not *"Provide visual treatment such as
  colour or font style"* ([CUBE CSS: Composition](https://cube.fyi/composition.html),
  documented).

All four are discipline, not mechanism: nothing stops a rule from breaking them.

### 3.3 Layout: the layout manager re-learned as primitives

Flexbox and grid gave CSS real containers. Writers then rebuilt the toolkits' small set of
composable containers on top of them.

- **Every Layout** (Heydon Pickering and Andy Bell) names them *primitives*: Stack, Box, Center,
  Cluster, Sidebar, Switcher, Cover, Grid, Frame, Reel, Imposter, Icon, Container
  ([Composition](https://every-layout.dev/rudiments/composition/)). Its argument, documented:
  - On space: *"margin is really a property of the relationship between two proximate
    elements"*, so *"The trick is to style the context, not the individual element(s)"* ([The
    Stack](https://every-layout.dev/layouts/stack/)).
  - On reuse: namespacing a component's whole layout under its own name means *"we'll end up
    duplicating would-be shared styles. This is where most CSS bloat comes from"*; each
    primitive has *"a simple responsibility: 'space elements vertically', 'pad elements evenly',
    'separate elements horizontally'"*
    ([Composition](https://every-layout.dev/rudiments/composition/)).
  - On responsiveness: primitives are *"intrinsically responsive"*, and `@media` breakpoints are
    *"considered 'manual overrides'"*. A viewport query *"has no bearing on the actual available
    space"* for a component placed in a narrow column ([The
    Sidebar](https://every-layout.dev/layouts/sidebar/)). The Switcher flips between a row and a
    column at a threshold of its *container's* width using only `flex-basis` ([The
    Switcher](https://every-layout.dev/layouts/switcher/)).
  - The pages for Cluster, Cover, Grid, Center and Box are behind the book's purchase and were
    not read; §5 describes them from their common published form (**UNVERIFIED** against the
    book's text).
- **Braid** (SEEK's design system) makes it a rule: *"components should not provide surrounding
  white space. Instead, spacing between elements is owned entirely by layout components"*, with
  `Box`, `Stack`, `Inline`, `Columns` and a single white-space scale ([Braid:
  Layout](https://seek-oss.github.io/braid-design-system/foundations/layout), documented).
- **Max Stoiber**, *Margin considered harmful* (2020): *"Margin breaks component
  encapsulation"*, *"Margin makes components less reusable"*; *"move the responsibility of
  managing space to the parent-level"* ([mxstbr.com](https://mxstbr.com/thoughts/margin),
  opinion). Margins also collapse by rules that surprise ([Josh Comeau, The Rules of Margin
  Collapse](https://www.joshwcomeau.com/css/rules-of-margin-collapse/)).
- **Primer** has `Stack`, *"a layout component that creates responsive horizontal and vertical
  flows"* ([Primer Stack](https://primer.style/product/components/stack/)); **Radix Themes**
  has `Box`, `Flex`, `Grid`, `Container` and `Section` as its layout set ([Radix Themes:
  Layout](https://www.radix-ui.com/themes/docs/overview/layout)).
- **Container queries** (2023) let a component respond to the space it was given rather than
  the window: the web's version of a parent's allocation. **Subgrid** (2023) lets nested
  content align to its parent's tracks, which a form of label/field rows needs.

*Opinion:* the convergence is strong. Four independent design systems and a book arrive at the
toolkits' rule (the parent owns placement and space) and at roughly the same half-dozen
containers.

### 3.4 Tokens, layers and scope

- **Custom properties** (Baseline 2017) inherit through the DOM, which makes them the web's
  `DynamicResource` or `CompositionLocal`: a value set high in the tree is read below it, and
  changing it restyles everything that reads it. They are untyped unless registered with
  `@property` (Baseline July 2024).
- **Design tokens.** The W3C community group's format reached its first stable version,
  2025.10, on 2025-10-28 ([announcement](https://www.w3.org/community/design-tokens/2025/10/28/design-tokens-specification-reaches-first-stable-version/)).
  It is a JSON interchange format between design tools and code, with aliases: *"A design
  token's value can be a reference to another token"* ([Design Tokens
  Format](https://www.designtokens.org/tr/drafts/format/), documented). Design systems layer
  tokens the same way: Carbon's spacing scale uses *"multiples of two, four, and eight"*
  ([Carbon: Spacing](https://carbondesignsystem.com/elements/spacing/overview/)).
- **Cascade layers** (`@layer`, Baseline March 2022): *"once the layer order has been
  established, specificity and order of appearance are ignored"* ([MDN:
  @layer](https://developer.mozilla.org/en-US/docs/Web/CSS/@layer), documented). This ends
  specificity wars between tiers by declaration rather than by selector weight.
- **`@scope`** (Baseline March 2026, "newly available") bounds a rule to a subtree, including
  *"donut"* scopes that stop at an inner boundary ([MDN:
  @scope](https://developer.mozilla.org/en-US/docs/Web/CSS/@scope)). It is the first native
  answer to "this component's rules must not reach into the component nested inside it".
- **CSS Modules** hash class names so they are *"scoped locally by default"*
  ([css-modules](https://github.com/css-modules/css-modules)). The project rejected them because
  a theme needs stable class names ([typescript.md](../../standards/typescript.md), Styling).

### 3.5 Where styles live: CSS-in-JS and utility-first

- **CSS-in-JS** began as an answer to seven problems of CSS at scale: *"Global Namespace,
  Dependencies, Dead Code Elimination, Minification, Sharing Constants, Non-deterministic
  Resolution, Isolation"* (Christopher Chedeau, [React: CSS in
  JS](https://speakerdeck.com/vjeux/react-css-in-js), November 2014). Every one of them is real.
- **Its runtime form declined.** React's working group explained that inserting rules while
  rendering is costly: *"When you add or remove any CSS rules, you more or less have to reapply
  all rules that already existed to all nodes that already existed"* ([reactwg/react-18
  #110](https://github.com/reactwg/react-18/discussions/110), documented). One team measured a
  component's render at 54.3 ms with Emotion and 27.7 ms after moving to static CSS, *"a 48%
  decrease"* (Sam Magura, [Why We're Breaking Up with
  CSS-in-JS](https://dev.to/srmagura/why-were-breaking-up-wiht-css-in-js-4g9b), 2022; their
  measurement, on an M1 Max). In March 2025 styled-components' maintainer put it in maintenance
  mode: *"For new projects, I would not recommend adopting styled-components or most other
  css-in-js solutions"* ([Open
  Collective](https://opencollective.com/styled-components/updates/thank-you)). The project
  already bans runtime CSS-in-JS for this reason ([typescript.md](../../standards/typescript.md)).
- **Utility-first** (Tailwind) frames the choice as *"CSS that depends on HTML"*, where *"your
  HTML is restyleable, but your CSS is not reusable"*, against *"HTML that depends on CSS"*,
  where *"your CSS is reusable, but your HTML is not restyleable"* (Adam Wathan,
  [2017](https://adamwathan.me/css-utility-classes-and-separation-of-concerns/), opinion). The
  critique: utilities are *"atomic building blocks, sure, but so are CSS property-value
  pairs"*, long class strings make review harder, and conditional classes get repeated
  (Aleksandr Hovhannisyan, [Why I Don't Like Tailwind
  CSS](https://www.aleksandrhovhannisyan.com/blog/why-i-dont-like-tailwind-css/), opinion).
  *Opinion:* Tailwind's real gain is a fixed scale of spacing and colour values that the author
  cannot leave; tokens plus a lint give the same constraint without moving the look into every
  screen's markup, which is the opposite of what D245 §5 needs (one change, one place).

### 3.6 Behaviour separated from look: headless components

The Swing delegate split has returned as *headless* libraries: the library owns behaviour,
keyboard handling and accessibility, and the application owns every pixel.

- **React Aria**: *"Over 50 components with built-in behavior, adaptive interactions, top-tier
  accessibility, and internationalization out of the box, ready for your styles"*, *"style-free
  out of the box"* ([React Aria](https://react-spectrum.adobe.com/react-aria/), documented).
- **Radix Primitives**: *"a low-level UI component library with a focus on accessibility,
  customization and developer experience"*, for the patterns *"documented by WAI-ARIA"* ([Radix
  Primitives](https://www.radix-ui.com/primitives/docs/overview/introduction)).
- **Ariakit**: *"unstyled, primitive components"* ([Ariakit](https://ariakit.org/)).

The platform is absorbing some of this: `<dialog>` (2022), `inert` (2023) and the popover
attribute (2025) give modal and overlay behaviour, focus trapping and top-layer stacking
without a library
([MDN: inert](https://developer.mozilla.org/en-US/docs/Web/HTML/Global_attributes/inert),
[MDN: Popover API](https://developer.mozilla.org/en-US/docs/Web/API/Popover_API)).

### 3.7 Focus and directional navigation

The toolkits for remotes and keyboards have a focus engine: on Android TV, *"The D-pad
transfers focus from one object to the nearest"* item ([TV
navigation](https://developer.android.com/training/tv/start/navigation)); Apple describes focus
groups where Tab moves between areas and arrows move *"among items in the same focus group"*
([HIG: Focus and
selection](https://developer.apple.com/design/human-interface-guidelines/focus-and-selection),
documented).

The web has sequential Tab order and nothing else in the platform today:

- CSS Spatial Navigation stopped at a Working Draft of 26 November 2019
  ([css-nav-1](https://www.w3.org/TR/css-nav-1/)); its WICG repository was *"archived by the
  owner on Mar 23, 2026"* ([WICG/spatial-navigation](https://github.com/WICG/spatial-navigation)).
- `focusgroup`, an attribute for arrow-key navigation in composite widgets, entered an origin
  trial in Chromium in March 2026: *"Authors routinely hand-code 'roving tabindex' logic"*, and
  *"One attribute replaces hundreds of lines of boilerplate"* ([Chrome: Request for developer
  feedback: focusgroup](https://developer.chrome.com/blog/focusgroup-rfc), [Open UI
  explainer](https://open-ui.org/components/scoped-focusgroup.explainer/), documented). Mozilla
  and WebKit positions are listed as pending in the explainer.
- Libraries fill the gap for televisions ([Norigin Spatial
  Navigation](https://github.com/NoriginMedia/Norigin-Spatial-Navigation),
  [BBC LRUD](https://github.com/bbc/lrud)).

## 4. The contrast

### 4.1 Lessons, and whether the web has re-learned them

| Lesson from the toolkits | Web equivalent | Re-learned? | What is still missing |
| --- | --- | --- | --- |
| The parent places children; a child never positions itself | Flexbox (2015), grid (2017) | Yes, mechanism exists | Nothing stops any rule from setting `margin`, `position` or `width` on anything |
| The parent owns the space between children | `gap` in flex (2021); Stack, Braid | Yes, since 2021 | `margin` is still everyone's default habit; no enforcement |
| Size negotiation: preferred, minimum, stretch | `flex-basis`/`grow`/`shrink`, `minmax()`, intrinsic sizes | Mostly | `min-width: auto` on flex and grid items: a long word widens the page unless the author knows to write `minmax(0, 1fr)` or `min-width: 0` |
| A child sizes to the space it is given | Container queries (2023) | Yes, recent | Few authors use them; viewport media queries remain the habit |
| A few composable containers, not one universal one | Every Layout, Braid, Primer, Radix Themes | Yes, as libraries and conventions | No standard set; each project rebuilds them |
| Spacing from a small named scale, font-relative | `rem`/`em`, tokens | Yes | Nothing enforces the scale; one-off lengths creep in |
| A narrow, typed theme seam | Custom properties (2017), `@property` (2024), Design Tokens (2025) | Yes | Custom properties are untyped by default; nothing stops a rule from bypassing them |
| A component's style cannot leak out or be reached into | Shadow DOM, CSS Modules, `@scope` (2026), BEM by convention | Partly | Any selector can still reach any element; the cascade is global by default |
| Tiers override in a fixed order, not by selector weight | `@layer` (2022) | Yes | Little adopted (*opinion*) |
| Behaviour and accessibility separate from look | Headless libraries; `<dialog>`, `inert`, popover | Yes | Complex widgets (combobox, grid, menu) still need a library or a lot of code |
| A focus engine with directional navigation | none in the platform | **No** | Spatial navigation stalled; `focusgroup` only in trial |
| A visual builder or preview of every component | Storybook and similar, as add-ons | Partly | Not part of the platform; a project must choose one |
| Constraint layout | none (CCSS 1999 and GSS did not take) | No, and *opinion*: not needed | — |

### 4.2 Pitfalls the web still has, seen in this codebase

The web client is small and already disciplined: no component sets an outer margin except
`margin: 0` resets, and every component's classes carry its file's prefix. Its CSS still shows
each pitfall in small form (measured with `grep` over `web/apps/web/src/*.css` on 2026-10-05):

- **Styles that reach into other components.** `.app-shell__main label` and
  `.app-shell__main fieldset label` (`app-shell.css`) style every `<label>` on every screen. A
  form that wants its labels stacked has to override the shell, which is the owner's "fiddle with
  one part to make another look right". `subtitles.css` does the same for `label` and `select`
  inside it.
- **The same layout written again and again.** A wrapping row with a gap
  (`display: flex; flex-wrap: wrap; gap: …`) is declared in seven files (`app-shell.css` four
  times), and a gapped vertical list (`display: grid; gap; list-style: none`) in several more.
  `title.css` and `series.css` both declare `grid-template-columns: minmax(8rem, 14rem) 1fr`
  for the same image-beside-text head. These are Every Layout's Cluster, Stack and Sidebar,
  unnamed.
- **One-off lengths.** Over twenty declarations outside `tokens.css` carry a literal length
  (`9rem`, `8rem`, `14rem`, `18rem`, `4px`, …), though the standard says sizes *"appear only as
  `var(--…)` outside `tokens.css`"*. That rule is review-only, and review let these through.
- **Overflow fixes by hand.** `play.css` needs `minmax(0, 1fr)` and `subtitles.css`
  `min-width: 0` so a long track name does not widen the page. Correct, but each screen must
  rediscover it; a Stack or Sidebar primitive would carry it once.
- **One global stylesheet.** Vite bundles every component's CSS into the page, so every class is
  global and, between equal selectors, the order of imports decides. Layers would make that
  order explicit.
- **Tokens without a scale.** `tokens.css` has one space value, `--space-page`, and components
  derive others as `calc(var(--space-page) / 2)` and `/ 4`. A scale with names would make those
  steps choices from a list rather than arithmetic.

## 5. Recommendations

Each recommendation gives its reason. Those that add a dependency or change a gate are the
owner's decision (AGENTS.md rules 2 and 3) and are offered as options.

### R1. Five tiers, ordered by cascade layers

`@layer tokens, base, layout, components, screens;` declared once, in that order, with every
CSS file placing its rules in one layer.

- **tokens:** custom properties only.
- **base:** element defaults (`body`, headings, links, form controls, the focus ring).
- **layout:** the primitives of R2, and nothing else.
- **components:** each reusable component's own classes.
- **screens:** each screen's own classes, composing the tiers above.

**Reason:** the toolkits' tiers (palette, style, layout, widget, window) become explicit, and a
later tier wins by declaration, not by selector weight (§3.4). Supported since March 2022.

### R2. A small set of layout primitives, as React components

Start with the ones the current screens already hand-write (§4.2):

| Primitive | Job | Replaces today |
| --- | --- | --- |
| `Stack` | Children in a column, `gap` from the space scale | the gapped lists in `people.css`, `series.css`, `your-devices.css`, `credentials-form.css` |
| `Cluster` | Children in a row that wraps, `gap` from the scale, alignment | the seven flex-wrap rows, `FormRow` |
| `Sidebar` | A narrow side and a wide main that stack when the main would be too narrow | `title__head`, `series__head`, `series__episode` |
| `Grid` | Equal tiles that fill the width, a minimum tile size | `poster-grid` |
| `Center` | A readable maximum width with side gutters | the page body (not yet present) |
| `Switcher` | A row of equal parts that becomes a column below a container width | settings and System panels, later |

Later, when the screens need them: `Reel` (a horizontal row that scrolls, for home rows), `Frame`
(an aspect-ratio box for artwork), `Cover` (vertically centred content, for sign-in).

Rules for every primitive:
- Its props are its layout choices, typed: `space` takes a name from the space scale
  (`"small" | "medium" | …`), never a length. `tsc` then enforces the scale.
- It lays out its direct children only and sets nothing on their insides.
- It carries the overflow fix (`min-width: 0`, `minmax(0, 1fr)`) so screens never need it.
- It responds to its container (`flex-basis`, `auto-fill`, container queries), never to the
  viewport. Viewport media queries belong to the app shell only.

**Reason:** this is the layout-manager protocol of §2.2 in CSS, in the form four design systems
and Every Layout converged on (§3.3). A layout fix made in a primitive reaches every screen that
uses it; one made in a screen reaches only that screen.

### R3. The parent owns space; components have no outer margin

A component sets nothing outside its own box: no `margin`, no `position` relative to siblings,
no width that assumes its context. Space between things is the `gap` of the primitive that holds
them. Inner padding is the component's own business.

**Reason:** this is the rule that makes a change local. A component with a margin changes the
layout of whatever it is placed in (§3.3: Every Layout, Braid, Stoiber). The codebase already
almost follows it (§4.2), so stating it costs nothing now.

### R4. Tokens in two tiers: a scale, then roles

- **Scale tokens:** a short space scale (for example five or six steps, font-relative in `rem`),
  a type scale, a colour palette, radii, durations.
- **Role tokens:** what a component reads: `--color-text`, `--color-surface`,
  `--color-problem`, `--space-gutter`, the focus ring of [D189](../../decisions/working-notes.md).
  A role is defined from the scale.
- Components and screens read **roles and space steps**, never palette colours or raw lengths.
- Keep them in `tokens.css`. Do not adopt the Design Tokens JSON format until a second consumer
  (a design tool, the television client) needs the same values; then the JSON becomes the source
  and `tokens.css` is generated from it.

**Reason:** the narrow typed seam that lasted in the toolkits (§2.4: `QPalette`, `ThemeData`,
Compose). With one skin (D245 §2) the tokens *are* the skin: changing the look of the whole
client is editing role and scale values, and nothing else moves.

### R5. A component owns its look completely; screens compose, they do not restyle

- A component's CSS selects only its own prefixed classes. No element selectors into its
  children (`.x label`), no selectors into another component's classes.
- A shared component accepts **no `className` or `style` prop**. If one screen needs it to look
  different, the component gains a named variant (`<Button kind="quiet">`), which every screen
  can then use. `tsc` enforces the absence of the prop.
- A screen's CSS styles the screen's own classes, and arranges components with primitives.

**Reason:** Qt's own verdict on style sheets (§2.4) is that selectors reaching into widgets
*"couple the design to implementation details"* and scatter a widget's styling. The variant is
WPF's named style: an override that has a name, lives with the component, and shows up in
review. This is what lets a screen change its look (its own CSS, its own composition) without
touching another screen, and a component change its look once for everyone.

### R6. Behaviour: native elements first, a headless library only for the hard widgets

- Build the simple components on native elements: `<button>`, `<dialog>`, `<details>`,
  `<select>`, the popover attribute, `inert` for what sits under a modal. Each brings keyboard
  and accessibility behaviour from the browser.
- For widgets the platform lacks (a combobox, a menu button with arrow keys, a grid of tiles
  with roving focus), the options, each the owner's choice under rule 3:

| Option | Job it would do | Licence, size (npm, 2026-10-05) |
| --- | --- | --- |
| None: write our own | Roving focus, menus, combobox by hand, tested with the keyboard walk | — |
| `react-aria-components` | Unstyled, accessible components with full keyboard and i18n behaviour | Apache-2.0; 7 direct dependencies; 6.6 MB unpacked |
| Radix Primitives | Unstyled WAI-ARIA patterns, one package per component | MIT; `@radix-ui/react-dialog` alone has 15 direct dependencies |
| `@ariakit/react` | Unstyled primitives, one package | MIT; 1 direct dependency; 1.1 MB unpacked |

*Opinion:* none is needed for the next screens. Decide when the first combobox or tile grid is
built, and prefer the option whose focus model can be replaced on the television (R8).

**Reason:** the delegate split of §2.4 and §3.6: behaviour in one place, look in another, so a
change of look cannot break the keyboard.

### R7. One focus-navigation mechanism, designed for the television

The web client's tile grids and rows need arrow-key navigation now (keyboard users, and
[D189](../../decisions/working-notes.md)'s focus rules) and spatial navigation later (the
television, [tv-client.md](../plans/tv-client.md),
which asks for a focus prototype before screens). Build one module for it, used by `Grid` and
`Reel`, with the roving-tabindex pattern; keep its interface narrow enough that a spatial
navigation library or `focusgroup`, if it ships in all engines, can replace its inside.

**Reason:** the web has no focus engine (§3.7), so this is the one lesson the platform has not
re-learned. One mechanism, not one per screen (AGENTS.md rule 11).

### R8. A specimen screen, for seeing every component at once

A development-only route that renders every primitive and component in each state and variant,
at a few container widths, in the real app. No dependency. (Storybook or Ladle would do the same
job as a separate tool; that is the owner's choice under rule 3, and not needed at this size.)

**Reason:** the toolkits had a designer that showed every widget in place (Interface Builder, Qt
Designer). When the look is iterated (D245 §5), the specimen screen shows what a token change
did everywhere in one view, and the layout tests of R10 can run against it.

### R9. Migrate the current client in one pass, before the redesign

Introduce the layers, the space scale and the primitives, then replace the hand-written
clusters, stacks and sidebars of §4.2, and move the shell's `label` rules into a `Field`
component. Behaviour does not change, so the existing screen tests should stay green unchanged.

**Reason:** the redesign then edits tokens, primitives and components, rather than every
screen's file laying itself out.

### R10. Enforcement

Every rule above should name its enforcer, as the project's culture requires. What can hold
each one:

| Rule | Mechanism | New dependency or gate change? |
| --- | --- | --- |
| Space only from the scale (R2) | `space` props typed as a union of names: `tsc` | No |
| No `className`/`style` on shared components (R5) | The props types: `tsc`; `style` on DOM elements banned by `no-restricted-syntax` | ESLint config: gate |
| No literal colour or length outside `tokens.css` (R4) | stylelint `color-no-hex`, `declaration-property-unit-allowed-list` or the `stylelint-declaration-strict-value` plugin | stylelint: dependency |
| No `margin` except in the layout layer (R3) | stylelint `property-disallowed-list`, with an override for the primitives' file | stylelint |
| No selectors into other elements or components (R5) | stylelint `selector-max-type: 0`, `selector-max-compound-selectors`, `selector-max-specificity`; a check that a file's classes carry its filename prefix | stylelint, plus a small custom rule or a Python check |
| Every rule in a layer (R1) | a check that each CSS file opens with `@layer` | Python check or stylelint |
| No viewport media queries outside the shell (R2) | stylelint `at-rule-disallowed-list` with an override for the shell | stylelint |
| No horizontal overflow, no overlapping controls | Playwright on the specimen screen and each screen at several widths: `scrollWidth <= clientWidth`, pairwise bounding boxes of interactive elements do not intersect | No (Playwright is in use) |
| Focus geometry | The D189 tests already planned in [testing-strategy.md](../plans/testing-strategy.md) | No |

Two routes for the CSS rules, both the owner's: **stylelint** (MIT, 35 direct dependencies,
about 1 MB unpacked on 2026-10-05) does all of them with its built-in rules except the filename
prefix; **a Python check in `scripts/`** next to `check_source.py` needs no dependency but parses
CSS with regular expressions and is a gate change. *Opinion:* stylelint, because a parser is
more reliable than patterns and its rules are documented elsewhere
([stylelint rules](https://stylelint.io/user-guide/rules)).

**Screenshot comparison is not recommended as a gate.** Playwright documents that *"Browser
rendering can vary based on the host OS, version, settings, hardware"* ([Visual
comparisons](https://playwright.dev/docs/test-snapshots)), and every intended look change would
re-record goldens, which AGENTS.md rule 5 treats as weakening a test. Geometry assertions (above)
state what must stay true without freezing pixels. Screenshots of the specimen screen are still
useful for review.

### R11. What not to adopt

- **A CSS framework (Bootstrap) or utility-first CSS (Tailwind).** Both move layout decisions
  into each screen's markup, so a look change touches every screen (§3.5); the scale they
  enforce, tokens and a lint enforce too. Tailwind is already banned by the standard.
- **Runtime CSS-in-JS.** Already banned; the ecosystem has left it (§3.5).
- **A constraint solver.** The platforms that had one moved the everyday work to stacks (§2.3).
- **Relying on `@scope` now.** It became Baseline only in March 2026 and the Cast receiver's and
  televisions' browsers are older (**UNVERIFIED**: their engine versions were not checked). The
  prefix convention plus a lint does the same job today.

## 6. Open questions for the owner

1. **Is D138 §4's user-facing theming seam still wanted, now that D245 §2 asks for one skin?**
   If yes, class names and tokens are a public surface and must stay stable, which keeps the
   standard's case against CSS Modules. If no, scoped or hashed class names become an option,
   and the token set can change freely.
2. **stylelint, or a Python check?** Either enforces R3 to R5 (R10). stylelint is a new
   dependency (rule 3); the check is a change to `scripts/` (rule 2).
3. **Which browsers are the floor?** It decides whether container queries (2023), popover
   (2025) and `@scope` (2026) can be used, and depends on what the Cast receiver and her laptop
   run.
4. **Behaviour library: none for now, and which one when the first hard widget comes?** (R6.)
5. **Should keyboard arrow navigation in grids and rows be built in Cut A**, or left for the
   television milestone? (R7.)
6. **Specimen screen in the app, or Storybook/Ladle?** (R8.)

## 7. Not tried

- No primitive was prototyped and nothing was measured for render or layout cost; the
  recommendations rest on documentation and the convergence of design systems, not on a test in
  this client.
- stylelint was not installed or run against `web/`, so how many existing lines each proposed
  rule would flag is unknown.
- Every Layout's pages for Cluster, Cover, Grid, Center and Box are behind its purchase and were
  not read (§3.3).
- Material 3's and Spectrum's token documentation, Nathan Curtis's *Space in Design Systems* and
  Netflix's TV UI article are rendered by script or blocked, and were not read. The State of CSS
  survey's usage figures are rendered by script, so no adoption numbers are given.
- The browser engines in the Cast receiver, Google TV and older laptops were not checked against
  the features in §3.1.
- Web components and Shadow DOM, Delphi's FireMonkey, Android's View-system `ConstraintLayout`
  performance, and Auto Layout's performance history were not examined.
- No interface was built in any of the toolkits of §2; their failures are as their own
  documentation and authors describe them.
