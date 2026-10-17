> **ABOUTME:** What the field's leading practitioners say about interaction, flow and ease of use,
> distilled into principles Orpheum's screens can be tested against, with where they disagree and
> how to iterate with two users on a staging server. Input to M5's UI principles document.

# UX Principles: Interaction, Flow and Ease of Use, from the Practitioners

- **Date:** 2026-10-09.
- **Requested by** the owner, for M5: *"Once we get into the UI design part of M5, UX flow and
  ease-of-use becomes the primary driver, which means that we need a solid set of best practices
  and principles, derived from the work and writings of the top practitioners in the industry."*
  [D245](../../decisions/working-notes.md) §4 asks for a UI principles document; this is the
  interaction half of its input.
- **Builds on, and does not repeat:**
  [ui-design-practice-2026-10-05.md](ui-design-practice-2026-10-05.md) (the visual practice:
  navigation placement, grids, type, colour, motion, loading and empty states, competitors' looks
  and what users say), [ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md) (how the code
  is built so the look can change), [ui-competitors.md](ui-competitors.md) (what works and fails by
  surface, Watch Together's patterns) and [ui-sentiment.md](ui-sentiment.md). The project's own
  engineering principles are in [principles/README.md](../../principles/README.md); several of
  them are already UX rules, and are cited where they are.
- **Scope:** behaviour, not looks. How a person forms a picture of the product, moves through it,
  learns what happened, recovers from mistakes, and does things together with someone elsewhere.
- **Sources:** primary where possible.
  - **Read on the web, 2026-10-09:** Nielsen Norman Group articles; Bruce Tognazzini's *First
    Principles of Interaction Design (Revised & Expanded)*; Ben Shneiderman's *Eight Golden
    Rules* (sixth edition text, on his site); Larry Tesler's own page on the Law of Conservation
    of Complexity; Jon Yablonski's *Laws of UX*; Bret Victor's *Magic Ink*; Dieter Rams's ten
    principles (Vitsœ); Luke Wroblewski's *Obvious Always Wins*; Joel Spolsky's *Joel Test*;
    Apple's Human Interface Guidelines (SharePlay, Feedback, Undo and redo, Alerts, Modality,
    Onboarding, Launching, Remotes, Playing video, Searching, Going full screen), read through
    their JSON data endpoints; the Netflix TechBlog's *Artwork Personalization* (through
    archive.org); the ACM abstracts of Geerts et al. (CHI 2011) and Weisz et al. (CHI 2007),
    through archive.org.
  - **Cited from the book, not re-read for this report** (marked **[book]**): Don Norman, *The
    Design of Everyday Things* (revised edition, 2013); Alan Cooper et al., *About Face* (fourth
    edition, 2014); Steve Krug, *Don't Make Me Think, Revisited* (2014) and *Rocket Surgery Made
    Easy* (2010); Jef Raskin, *The Humane Interface* (2000); Kathy Sierra, *Badass: Making Users
    Awesome* (2015); Luke Wroblewski, *Mobile First* (2011) and *Web Form Design* (2008). Where a
    book's idea is stated, its Wikipedia summary was read to check the attribution
    (*The Humane Interface*, *The Design of Everyday Things*, *Don't Make Me Think*).
- **Cache:** every fetched page is under `/yoloai/cache/http/uxp/` (`<name>.raw` as fetched,
  `<name>.txt` as extracted; Apple's as JSON, extracted by `applejson.py`).
- **Legend:** **[doc]** read in a primary source for this report; **[book]** as above; **[prior]**
  established in an earlier research document of this tree, cited; **Judgement:** this
  researcher's reasoning, not a source's.

## 1. What the field agrees on, in one paragraph

Read side by side, the practitioners converge far more than their vocabularies suggest. Norman's
*conceptual model*, Cooper's *mental model versus implementation model*, Nielsen's *match between
the system and the real world* and Krug's *don't make me think* are one idea: the product should
work the way the person already believes it works. Nielsen's *visibility of system status*,
Shneiderman's *informative feedback*, Norman's *feedback* and Tognazzini's *status mechanisms* are
one idea. Nielsen's *user control and freedom*, Shneiderman's *easy reversal* and *internal locus
of control*, Raskin's universal undo and Apple's forgiveness are one idea. Tesler's conservation
of complexity, Cooper's *excise* and Victor's *interaction considered harmful* are one idea: the
work the person does that does not serve their goal is the designer's failure, and the software
should absorb it. The disagreements (§4) are real but narrow: about modes, about confirmations,
about how many ways to do one thing, and about how much to hide.

## 2. The practitioners, and what each contributes here

| Who | Work | What it adds to a media server with Watch Together |
| --- | --- | --- |
| Don Norman | *The Design of Everyday Things* [book] | the vocabulary: affordance, signifier, mapping, feedback, constraint, conceptual model; slips versus mistakes; the gulfs of execution and evaluation |
| Jakob Nielsen, NN/g | ten heuristics; articles [doc] | the checklist a reviewer runs; the research numbers (hidden navigation, five users, response times) |
| Bruce Tognazzini | *First Principles of Interaction Design* [doc] | discoverability (*"If the user cannot find it, it does not exist"*), protecting work, latency, continuity over consistency |
| Jef Raskin | *The Humane Interface* [book] | modes and quasimodes, habituation, monotony, universal undo instead of warnings |
| Alan Cooper | *About Face* [book] | goals over tasks, posture (sovereign and transient), excise, perpetual intermediates |
| Steve Krug | *Don't Make Me Think*; *Rocket Surgery Made Easy* [book] | people scan and satisfice; the trunk test; cheap, frequent testing |
| Larry Tesler | the Law of Conservation of Complexity [doc]; modeless editing | who carries the irreducible complexity |
| Ben Shneiderman | *Eight Golden Rules* [doc]; direct manipulation | closure, reversal, locus of control |
| Jon Yablonski | *Laws of UX* [doc] | Hick, Fitts, Jakob, Doherty, Tesler, goal-gradient, choice overload in one place |
| Luke Wroblewski | *Mobile First*; *Web Form Design* [book]; *Obvious Always Wins* [doc] | phone first; what is hidden is unused; forms |
| Kathy Sierra | *Badass* [book] | the user wants to be good at what they do, not at your product |
| Bret Victor | *Magic Ink* [doc] | infer from history and context before asking |
| Dieter Rams | ten principles [doc] | *"as little design as possible"*; understandable; honest |
| Apple | Human Interface Guidelines [doc] | the playback rules, SharePlay's co-watching rules, alerts, undo, modality, remotes |
| Netflix | TechBlog; Gomez-Uribe and Hunt, *ACM TMIS* 2015 | choosing time, artwork as evidence |
| Geerts et al.; Weisz et al. | CHI 2011; CHI 2007 [doc, abstracts] | voice versus text when watching together |

## 3. The principles

Each principle has **the rule**, **why** (the sources that converge on it), **in Orpheum** (what
it means on this app's screens), and **check** (how a reviewer tests a design against it). Twenty-
five, in nine groups. A principle that restates one already in
[principles/README.md](../../principles/README.md) says so and does not re-argue it.

The screens named are: **Home**; the **library grid**; the **title page** (film, series, season);
the **player**; **Watch Together** (the host's screen, the invite, the viewer's join card, the
shared player with voice); and **Manage** (libraries, jobs, people, settings).

### 3.1 Conceptual model and navigation

#### U1. The product has one conceptual model, told in the viewer's words

- **Rule.** Every screen a viewer sees speaks of things a viewer already knows: films, series,
  episodes, libraries, people watching together. Implementation nouns (file, edition, rendition,
  ladder, chunk, prepare, transcode, node, session token) appear only in Manage, and there with a
  sentence saying what they are for.
- **Why.** Norman: the designer's model reaches the user only through the *system image*, so the
  system image must carry the right model [book]. Cooper: products that reflect the
  *implementation model* rather than the user's *mental model* are hard to use [book]. Nielsen's
  heuristic 2: *"speak the users' language… rather than internal jargon"* [doc]
  ([NN/g](https://www.nngroup.com/articles/ten-usability-heuristics/)). NN/g on mental models:
  *"Mismatched mental models are common, especially with designs that try something new"* [doc]
  ([NN/g](https://www.nngroup.com/articles/mental-models/)).
- **In Orpheum.** A film with two files is one film with a choice of versions, not two items. A
  Watch Together session is described as *"watching with Anna"*, not as a session or a
  membership. A quality step-down ([D253](../../decisions/working-notes.md)) is *"Switched to a
  smaller version so it keeps playing"*, not *"rendition 480p"*. The viewer who arrives by an
  invite link never sees the words *account*, *role* or *library*.
- **Check.** List every noun and verb on the screen. Each is either one a viewer would say aloud
  or is on a Manage screen with an explanation. A word that means two things anywhere in the app,
  or two words for one thing, fails.

#### U2. Navigation is visible, and every screen answers "where am I, and where can I go?"

- **Rule.** The places a person goes often are always in view, with text labels; the current
  place is marked; Search is reachable from every screen. Nothing frequent lives behind a hover,
  a long press or an unlabelled icon.
- **Why.** Tognazzini: *"Make navigation visible"*; *"Offer users stable perceptual cues for a sense
  of 'home'"*; *"If the user cannot find it, it does not exist"* [doc]
  ([asktog](https://asktog.com/atc/principles-of-interaction-design/)). NN/g's quantitative
  study: *"Discoverability is cut almost in half by hiding a website's main navigation"* [doc]
  ([NN/g](https://www.nngroup.com/articles/find-navigation-mobile-even-hamburger/)).
  Wroblewski: engagement *"plummeted"* when Polar hid its sections behind a toggle menu, and rose
  for Facebook and Redbooth when they moved from a hamburger to a tab bar [doc]
  ([LukeW](https://www.lukew.com/ff/entry.asp?1945)). Krug's *trunk test*: dropped on any page,
  a person should be able to say what site this is, what page they are on, what the major
  sections are, what their options are here, where they are in the scheme of things, and how to
  search [book]. NN/g: search must be *"available from every page"*, because *"you cannot predict
  where users will be when they decide they are lost"* [doc]
  ([NN/g](https://www.nngroup.com/articles/search-visible-and-simple/)).
- **In Orpheum.** The menu split of [D251](../../decisions/working-notes.md) §5 (watching, Manage,
  the person's own) already follows this; the placement question (left or top) is settled in
  [ui-design-practice-2026-10-05.md](ui-design-practice-2026-10-05.md) §5.1 item 5. What this
  adds: the always-visible Watch Together indicator is a navigation landmark, not decoration; a
  title page names its library and, for an episode, its series and season, each a link; Plex's
  hidden long-press options are the failure to avoid ([prior], that report §4.1).
- **Check.** Run Krug's trunk test on a screenshot of each screen with the address bar cropped. Any
  of the six questions unanswerable fails. Then list each action on the screen: any that needs
  hover or long press to be found, with no visible equivalent, fails.

#### U3. Back goes back, and coming back finds things as they were left

- **Rule.** The browser's Back, a phone's back gesture and Esc each undo the last move through the
  app. A page reached again (by Back, a reload or a shared address) shows the same item, filter,
  sort and scroll position. Closing the player returns to where the person came from, with the
  played item in view.
- **Why.** Apple, on the remote's Back: *"open the parent of the current screen"* [doc]
  ([HIG, Remotes](https://developer.apple.com/design/human-interface-guidelines/remotes)); on
  launching: *"Restore the previous state… Avoid making people retrace steps… scroll the view to
  people's most recent position"* [doc]
  ([HIG, Launching](https://developer.apple.com/design/human-interface-guidelines/launching)).
  Tognazzini: *"Users should be able to log off at work, go home, and take up exactly where they
  left off"* [doc]. Nielsen's heuristic 3, a *"clearly marked 'emergency exit'"* [doc]. Exiting
  playback lands on the title page [prior] (ui-design-practice §2.1).
- **In Orpheum.** The library grid's sort and filter are in the address, so Back from a title page
  restores the grid at the poster that was opened, not at the top. A Watch Together viewer's
  reload returns them into the session (the membership already survives it, D267). Esc closes a
  menu or dialog before it leaves fullscreen, and leaves fullscreen before it does anything else.
- **Check.** For each screen: open an item, press Back; reload; paste the address into a new tab.
  Any lost filter, scroll position or selection fails.

#### U4. The shortest path to the picture is the design's first measure

- **Rule.** The things people come to do (resume, play the next episode, find a named film, start
  watching with someone) each have a click count with a target, and nothing stands between a
  chosen item and its first frame that the person did not ask for.
- **Why.** [D138](../../decisions/working-notes.md) §1 made click depth and focus latency the two
  numbers this project's clients are judged on, from Plex's forum. Apple: *"Avoid asking people if
  they want to resume playback"*; *"avoid displaying splash screens, detail screens, intro
  animations, or any other barriers that make it take longer to reach content"* [doc]
  ([HIG, Playing video](https://developer.apple.com/design/human-interface-guidelines/playing-video)).
  Cooper: navigation that does not serve the goal is *excise*, and *"navigation is excise"* is one
  of his named patterns [book]. Hick's law: decision time grows with the number of choices [doc]
  ([Laws of UX](https://lawsofux.com/hicks-law/)).
- **In Orpheum.** Continue Watching to playing is one action ([D188](../../decisions/working-notes.md)
  §1). A film's title page has one primary button, *Play* or *Resume at 41:12*. A viewer's invite
  link to watching is a name and one *Join* (already so in
  [watch-together.md](../plans/watch-together.md)). The host starting Watch Together from a title
  page, rather than first opening a separate Watch Together screen, is a candidate to measure.
- **Check.** A table of the top tasks with today's click count and the target, kept with the
  screens and re-measured each iteration (§5). A change that adds a step to a top task needs a
  stated reason.

### 3.2 Feedback and system status

#### U5. Every action answers at once, and every wait says what it is waiting for

- **Rule.** A press is acknowledged visibly within 100 ms. An operation that will take longer than
  a second shows that it is under way, in place; one longer than ten seconds shows progress and a
  way to stop. A wait never looks like a hang.
- **Why.** Nielsen's heuristic 1: *"keep users informed about what is going on, through
  appropriate feedback within a reasonable amount of time"* [doc]. Shneiderman's rule 3: *"For every
  user action, there should be an interface feedback"* [doc]
  ([Golden Rules](https://www.cs.umd.edu/~ben/goldenrules.html)). The Doherty threshold:
  productivity *"soars"* below 400 ms [doc] ([Laws of UX](https://lawsofux.com/doherty-threshold/)).
  Tognazzini: *"Keep users informed when they face delay"*, and *"Make it faster to begin with"*
  [doc]. The 0.1 s, 1 s and 10 s limits and the skeleton rules are in [prior]
  (ui-design-practice §2.8).
- **In Orpheum.** *Play* changes state the moment it is pressed, before the first segment arrives;
  after two seconds the black screen gets a spinner (Apple's rule [prior]). A viewer's player
  that is buffering says so to the others by name (Jellyfin's unused *"{0} is buffering…"* string
  is the failure [prior], ui-competitors §4). A library scan and a preparation show progress in
  Manage and on the title page they affect.
- **Check.** Throttle the network in the browser and press every button on the screen: anything
  with no visible change within 100 ms fails; any wait over a second with no in-place indicator
  fails.

#### U6. Feedback is as loud as what it reports, and sits beside what it describes

- **Rule.** Routine outcomes are shown quietly, in place. Interruptions (a dialog, a sound, a
  notice over the video) are kept for what needs the person to act or to know now. Status lives
  next to the thing it describes, not in a toast that vanishes or a page elsewhere.
- **Why.** Shneiderman: *"For frequent and minor actions, the response can be modest, whereas for
  infrequent and major actions, the response should be more substantial"* [doc]. Apple: *"match
  the significance of the information to the way it's delivered"*; *"Consider integrating status
  feedback into your interface… near the items it describes"*; *"Avoid using an alert merely to
  provide information"*; and *"Avoid showing an alert when your app starts"* [doc]
  ([HIG, Feedback](https://developer.apple.com/design/human-interface-guidelines/feedback),
  [Alerts](https://developer.apple.com/design/human-interface-guidelines/alerts)).
  Tognazzini: *"Keep status information up to date and within easy view"* [doc].
- **In Orpheum.** A partner pausing is a short named line over the shared player (*"Karl paused at
  41:12"*), not a dialog. A film that is still being prepared says so on its poster and title
  page, not only in the jobs list. Voice dropping and reconnecting is shown on the voice control
  itself. The jobs list is for detail; the title page is where a person learns their film is not
  ready.
- **Check.** For each message the design can show: is it an interruption? If so, does it need an
  action or a decision now? An interruption that does not fails. A status shown only on a page
  other than where its subject is shown fails.

#### U7. An automatic decision is shown where it acts, with its reason, and can be overridden

- **Rule.** Already the project's [§3](../../principles/03-explain-the-decision.md)
  and [§15](../../principles/15-safe-visible-defaults.md).
  For the UI it means: where the app chose for the person (a quality, a subtitle track, an
  identification, a version), the screen says what was chosen and lets them choose otherwise in
  one step.
- **Why.** Tognazzini: *"Ensure status information is accurate"*, and the computer *"belong[s]"* to
  the user, who may make *"their own decisions"* [doc]. Victor argues the other side, that
  software should infer rather than ask (§4.4); this rule is how both are kept.
- **In Orpheum.** The player's info panel names why it transcodes (D188 §1). A subtitle track that
  came on by the series' remembered choice says so in the track menu. A film identified by the
  scanner shows *"identified from the file name"* beside a *Fix* action.
- **Check.** List what the screen shows that the app chose rather than the person. Each needs a
  visible source and a one-step override.

### 3.3 Control, forgiveness and modes

#### U8. Undo before confirmation; confirm only the irreversible, and say exactly what is lost

- **Rule.** An action that can be undone is done at once and offers undo where it happened. A
  confirmation is kept for what cannot be undone and is uncommon, names the thing and its
  consequence, and labels its buttons with verbs (*End for everyone*, *Keep watching*), never
  *OK*/*Yes*.
- **Why.** Shneiderman's rule 6: *"actions should be reversible. This feature relieves anxiety…
  and encourages exploration"* [doc]. Raskin wants universal undo and no warning screens, because
  people confirm out of habit [book] (summary checked:
  [Wikipedia](https://en.wikipedia.org/wiki/The_Humane_Interface)). NN/g: *"Do not use
  confirmation dialogs for routine actions"*; *"Be specific"*; buttons that *"summarize what will
  happen"*; *"go to great lengths to provide undo"* [doc]
  ([NN/g](https://www.nngroup.com/articles/confirmation-dialog/)). Apple: *"Avoid displaying
  alerts for common, undoable actions, even when they're destructive"*, and never *OK* as the
  default unless the alert is purely informational [doc] (HIG, Alerts). The project's
  [§2](../../principles/02-the-users-files.md) and
  [D143](../../decisions/working-notes.md) already chose *confirm → trash → empty*.
- **In Orpheum.** *Hide from Continue Watching* happens at once with *Undo* (D188 §1). Marking
  watched and unwatched: no confirmation. Withdrawing an invite link: no confirmation, with
  *Undo* while nobody has used the new state. *End session* is irreversible for the others, so it
  confirms: *"End watching for Anna? Her screen goes black and her link stops working."* Removing a
  library is the one Manage action whose confirmation states counts (titles, watch history kept or
  not).
- **Check.** For each destructive or consequential action: can it be undone? If yes, a
  confirmation fails the design. If no, the confirmation must name the item and the effect and use
  verb buttons, with no default on the destructive one.

#### U9. Nothing the person did is lost: their place, their choices, their half-typed input

- **Rule.** Positions, chosen tracks, volumes, filters and form input survive a reload, a dropped
  connection, a crash and a sign-in. A form that fails keeps what was typed.
- **Why.** Tognazzini: *"Ensure that users never lose their work… This principle is all but
  absolute"* [doc]. Raskin's first rule of interface design, in the book's own wording, that a
  computer shall not harm your work or, through inaction, allow it to come to harm [book].
  Shneiderman: users *"should not have to retype an entire name-address form"* [doc].
- **In Orpheum.** For a viewer, the work is their place in the film and their settings: their
  film volume and their partner's volume are kept per browser; a viewer who loses the network
  returns to the session at the group's position without a new link. For an admin, the
  new-library form keeps its fields after a refused path.
- **Check.** Make each choice on the screen, reload, kill the network for thirty seconds and
  restore it, and repeat in a second tab. Anything forgotten fails.

#### U10. Modes are few, signalled twice, and never trap

- **Rule.** Where the same input does different things depending on a state, that state is shown by
  at least two cues, its effect is named, and the way out is always visible. Modal dialogs are
  short, single-purpose and closable by Esc and an explicit button, and never stack.
- **Why.** NN/g: modes cause *mode slips*; *"we strongly recommend at least two visual indicators"*,
  and avoid modes *"entirely when mode slips can have unsafe outcomes"* [doc]
  ([NN/g](https://www.nngroup.com/articles/modes/)). Raskin and Tesler argue for modelessness
  outright (Tesler's website is still called *No Modes*) [book; doc]
  ([nomodes.com](https://www.nomodes.com/)). Apple: *"Always give people an obvious way to
  dismiss a modal view"*, *"keep modal tasks simple, short, and streamlined"*, *"Let people dismiss
  a modal view before presenting another one"* [doc]
  ([HIG, Modality](https://developer.apple.com/design/human-interface-guidelines/modality)).
- **In Orpheum.** **Hosting Watch Together is a mode**: while it is on, what the host plays plays
  for others too. That is the mode slip worth designing out: a host browsing to check something
  must not put a film on the viewer's screen by habit. The indicator in the menu is one cue; the
  second is on every *Play* button while hosting (*Play for both of you*), with a way to play only
  here (judgement: whether the host may watch something privately while hosting is a design
  question for the owner, U23). The player's controls-hidden state is a mild mode: any input
  shows controls rather than acting, except the documented keys.
- **Check.** List each state that changes what an input does. Each needs two cues and a visible
  exit. A dialog that opens another dialog fails.

#### U11. The person starts things; the app does not move, play or change on its own

- **Rule.** Nothing plays, advances, rearranges or changes layout unless the person did something
  or it was announced with a way to stop it. A layout change is opt-in
  ([D108](../../decisions/working-notes.md)).
- **Why.** Shneiderman's rule 7: experienced users *"don't want surprises or changes in familiar
  behavior"* [doc]. Apple on SharePlay: *"tie template changes to a person's explicit action"* and
  *"Let people opt in to immersion changes when they're mid-task"* [doc]
  ([HIG, SharePlay](https://developer.apple.com/design/human-interface-guidelines/shareplay)).
  D188 declined autoplaying previews and heroes. Tognazzini: *"Over time, strive for continuity,
  not consistency"* [doc].
- **In Orpheum.** The next episode starts after a visible countdown with *Cancel* and *Play now*;
  the "still watching?" rule of D188 stops the chain. Home rows never reorder while in view: a
  title that becomes watched leaves Continue Watching on the next visit, not under the pointer. In
  Watch Together, the host's next episode is announced to the viewer before it starts.
- **Check.** Leave each screen idle for five minutes with something changing on the server (a
  scan, a progress update, a partner's action). Anything that moves the focused item, changes
  what plays without a visible cause, or shifts layout under the pointer fails.

### 3.4 Efficiency and defaults

#### U12. Show, don't make them remember

- **Rule.** Options, recent choices and the things a person is likely to want are visible to be
  recognised; nothing requires remembering a value from one screen to type into another.
- **Why.** Nielsen's heuristic 6: *"The user should not have to remember information from one part of
  the interface to another"* [doc]. Shneiderman's rule 8 on short-term memory [doc]. Victor:
  software that does not learn from history *"dooms users to repeat it"* [doc]
  ([Magic Ink](http://worrydream.com/MagicInk/)).
- **In Orpheum.** Posters carry titles ([D188](../../decisions/working-notes.md) §1). Search shows
  recent searches (clearable, per Apple's privacy note [doc],
  [HIG, Searching](https://developer.apple.com/design/human-interface-guidelines/searching)). The
  track menu lists the series' remembered languages first. The host's invite screen shows each
  link's note (*"for Anna"*), so the host never has to remember which link is whose.
- **Check.** Walk each top task and note every value the person must carry in their head or type
  that the app already knew. Each is a finding.

#### U13. Good starting values remove questions; ask only what is consequential, when it first matters

- **Rule.** Already the project's
  [§15](../../principles/15-safe-visible-defaults.md)
  and [D200](../../decisions/working-notes.md). For flow: no setup step, preference screen or
  question appears before the moment its answer changes something.
- **Why.** NN/g: people *"rarely utilize fancy customization features, making it important to
  optimize the default"* [doc] ([NN/g](https://www.nngroup.com/articles/the-power-of-defaults/)).
  Apple: *"Postpone nonessential setup flows or customization steps. Provide reasonable default
  settings"* [doc] ([HIG, Onboarding](https://developer.apple.com/design/human-interface-guidelines/onboarding)).
  Apple on SharePlay: *"Defer nonessential steps to more natural moments"* [doc]. Tognazzini:
  *"Defaults should be 'intelligent' and responsive"* [doc].
- **In Orpheum.** The viewer's speaker question already comes after *Join* and no one waits for it
  (D246 §6). The microphone is asked for when the person presses *Talk*, not on joining. A new
  library asks for its name and folder; everything else takes its starting value and is shown on
  the library's page.
- **Check.** List every question the flow asks. For each: what changes if the person skips it?
  If nothing yet, it is asked too early.

#### U14. The software carries the complexity; the person does only the part that is theirs

- **Rule.** Every step a person takes that does not move them toward their goal is a cost the
  design must justify. Where a step can be inferred, inferred and shown (U7) beats asked.
- **Why.** Tesler: *"Every application has an inherent amount of irreducible complexity. The only
  question is: Who will have to deal with it—the user, the application developer, or the platform
  developer?"* [doc] ([Tesler](https://www.nomodes.com/larry-tesler-consulting/complexity-law)).
  Cooper's *excise*, the work a tool demands that does not serve the user's goal [book]. Victor:
  *"interaction should be used judiciously and sparingly"*; *"interactivity is the last resort"*
  [doc]. Kathy Sierra: people do not want to be good at your product, they want to be good at
  what it lets them do [book]. Rams: *"as little design as possible"* [doc]
  ([Vitsœ](https://www.vitsoe.com/gb/about/good-design)).
- **In Orpheum.** The viewer's goal is to watch with the host; device choice, network quality and
  codec are the software's to handle, and its only demand is the one gesture autoplay needs. The
  host's goal is "watch this with Anna": the link, the QR code and the note are the minimum.
  Headphones are a real requirement (D63) and are asked for in words, once, where voice starts.
- **Check.** Write each top task as the person's goal in one sentence, then list the steps. Mark
  each step *goal* or *excise*. Each excise step needs a reason the software cannot do it.

#### U15. Accelerators for the frequent, as aliases of what is visible

- **Rule.** Frequent actions get keyboard and media-key shortcuts, but each shortcut does exactly
  what a visible control does, and the visible control is enough on its own.
- **Why.** Nielsen's heuristic 7: *"Shortcuts — hidden from novice users — may speed up the
  interaction for the expert user"* [doc] ([NN/g](https://www.nngroup.com/articles/flexibility-efficiency-heuristic/)).
  Shneiderman's rule 2: *"features for experts, such as shortcuts"* [doc]. Cooper: most users are
  *perpetual intermediates*; design for them, with paths in for beginners and out for experts
  [book]. Raskin disagrees (§4.2).
- **In Orpheum.** The player's keys in [web-client.md](../plans/web-client.md) (Space, `k`, `j`,
  `l`, arrows, `f`, `m`, `c`) each mirror a control, and are bound to `KeyboardEvent.code`. The
  Media Session's handlers mirror the same play, pause and seek, and, per D268 §4, take the
  same path as the on-screen controls in Watch Together.
- **Check.** For each shortcut: name the visible control it mirrors and confirm identical effect
  (including what partners see in Watch Together). A shortcut with no visible counterpart fails.

#### U16. Progressive disclosure by frequency, at most two levels deep

- **Rule.** What people use often is on the first level; what they use rarely is one labelled step
  away; nothing is three steps away. The label of the step says what is behind it.
- **Why.** NN/g: disclose *"everything that users frequently need up front"*; *"designs that go
  beyond 2 disclosure levels typically have low usability"*; the step needs *"strong information
  scent"* [doc] ([NN/g](https://www.nngroup.com/articles/progressive-disclosure/)). Tognazzini:
  *"Use Progressive Revelation to flatten the learning curve"*, but *"Do not simplify by
  eliminating necessary capabilities"* and *"Any attempt to hide complexity will serve to increase
  it"* [doc]. Apple's player: menus *"don't take more than a step or two"* [doc].
- **In Orpheum.** The title page shows Play, versions and tracks; the file's path and streams are
  under a closed *File details* (D251 §7). The player shows play, time, tracks, volume and
  fullscreen; playback speed and the info panel are one step into a menu. Manage's library page
  shows name, folders and scan; matching rules and metadata sources are one labelled step in.
- **Check.** For each control, estimate its use frequency (from the owner's sessions, §5). Frequent
  and hidden fails; rare and on the first level is a candidate to move. Count levels: three fails.

### 3.5 Errors and recovery

#### U17. Prevent the error with a constraint before explaining it with a message

- **Rule.** Where a wrong input or an impossible action can be ruled out by the control itself, it
  is. A control that cannot act now says why where it is, rather than vanishing.
- **Why.** Nielsen's heuristic 5: *"the best designs carefully prevent problems from occurring in the
  first place"*, and the split into slips (*"unconscious errors caused by inattention"*) and
  mistakes (*"a mismatch between the user's mental model and the design"*) [doc]. Norman:
  *constraints* and *forcing functions*; *design for error* [book]. Shneiderman's rule 5: *"design
  the interface so that users cannot make serious errors"* [doc]. Postel's law, *"Be liberal in what
  you accept"* [doc] ([Laws of UX](https://lawsofux.com/postels-law/)).
- **In Orpheum.** The new-library form chooses a folder from a list of what the node sees (the
  owner's subfolder picker, m5.md), not a typed path. A time field in the player accepts
  `41:12`, `41m`, `2471`. *Play for both* is not offered for a film the guest's link cannot reach;
  it says *"Anna can't watch this one: it's in a library her invite doesn't cover"*.
- **Check.** For each error message the design has: could a control have made it impossible? For
  each disabled control: does it say why, in place?

#### U18. An error names what failed, why, and the one thing to do next, where it happened

- **Rule.** Already decided for the content (D188 §1: item, cause, one next action). For the flow:
  the message appears beside what failed, keeps the person's input (U9), never shows a code alone,
  and a recoverable failure recovers by itself while saying so.
- **Why.** Nielsen's heuristic 9: *"plain language (no error codes), precisely indicate the problem,
  and constructively suggest a solution"* [doc]. NN/g: error messages *"highly visible, provide
  constructive communication, and respect user effort"* [doc]
  ([NN/g](https://www.nngroup.com/articles/error-message-guidelines/)). Apple: titles that say
  *"what happened, the context in which it happened, and why"*; never *"Error"* [doc]. Tognazzini:
  *"Error messages should actually help"* [doc]. A network drop is a countdown, not an error page
  [prior] (ui-competitors §4).
- **In Orpheum.** *"Couldn't reach the server. Trying again in 5 s"*, then reconnected in place. A
  viewer whose link cannot carry the lowest quality is told so with both numbers (already in
  [watch-together.md](../plans/watch-together.md)). A refused microphone shows the browser's own
  words and a *Try again*.
- **Check.** Trigger each error the screen can show. It fails if it is away from its cause, loses
  input, shows only a code, or offers no next action.

### 3.6 Consistency

#### U19. Consistent with what people know: the platform, the field, and the app's own past

- **Rule.** Controls that look alike act alike, controls that act differently look different, and
  the app follows the conventions of the browser and of the players people use daily, unless a
  departure is measured to be better. A change to a familiar behaviour is opt-in for a release
  ([D108](../../decisions/working-notes.md)).
- **Why.** Jakob's law: *"Users spend most of their time on other sites"* [doc]
  ([Laws of UX](https://lawsofux.com/jakobs-law/)). Nielsen's heuristic 4 [doc]. Shneiderman's rule 1
  [doc]. Tognazzini: *"It is just [as] important to be visually inconsistent when things act
  differently as it is to be visually consistent when things act the same"*; and *"The most
  important consistency is consistency with user expectations"* (quoting Buxton) [doc]. Apple: a
  custom player that diverges *"slightly"* frustrates more than one that diverges a lot [prior].
  Apple on remotes: *"Prefer using standard gestures to perform standard actions"* [doc].
- **In Orpheum.** The skinned player keeps the browser player's layout and keys. A link looks like
  a link and navigates; a button looks like a button and acts. *Play* on a title page and *Play*
  while hosting look different because they do different things (U10). The same word appears for
  the same thing in Manage and in the viewer's screens.
- **Check.** Inventory the controls on the screen against the specimen screen
  ([ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md) R8): a control whose look matches
  one kind and whose behaviour matches another fails.

### 3.7 First use and empty states

#### U20. No tour: the empty screen teaches, and help comes in context

- **Rule.** No onboarding carousel or feature tour. A first-run or empty screen says what belongs
  there, why it is empty, and offers the one action that fills it. Tips appear when the thing they
  describe first becomes useful, once.
- **Why.** NN/g: *"we recommend professionals avoid creating app onboarding whenever possible and
  instead spend your resources making the UI more usable"*; deck-of-cards tutorials *"didn't
  improve task performance"* [doc] ([NN/g](https://www.nngroup.com/articles/mobile-app-onboarding/)).
  Apple: *"Ideally, people can understand your app or game simply by experiencing it"*; prefer
  *"context-specific tips"*; *"Teach through interactivity"* [doc] (HIG, Onboarding). Empty states
  say why and offer the next step [prior] (ui-design-practice §2.8). Krug: people *muddle through*
  rather than read instructions [book].
- **In Orpheum.** A new server's Home is *"Add your first library"* with the folder picker. An empty
  Continue Watching row is absent, not an empty row. The host's first Watch Together screen
  explains in one sentence what a link does and shows *Make a link*. The viewer's first *Talk*
  says *"Use headphones, or the film will echo back to Karl"* once, with the control.
- **Check.** Start from an empty database and an unknown browser. Every screen reached must say
  what to do next without external help; any instruction that is shown every time fails.

### 3.8 Input methods and accessibility

#### U21. Every action works by pointer, touch, keyboard and, later, remote, on targets sized for each

- **Rule.** Already required by D188 §2 and [D189](../../decisions/working-notes.md). For flow: no
  action depends on hover, right-click, long press or a gesture alone; the focus order follows
  the reading order; focus lands somewhere sensible after every navigation and every dialog.
- **Why.** Apple, for players: *"Support the interactions people expect, regardless of the input
  device"* [doc]. Tognazzini: *"Controls and other objects necessary for the successful use of
  software should be visibly accessible at all times"* [doc]. Plex's long-press-only options were
  a named complaint [prior]. Shneiderman's rule 2, *"universal usability"* [doc].
- **In Orpheum.** A poster's secondary actions (mark watched, hide) are on a visible menu button
  that also opens by keyboard, not only on hover. After *Join*, focus goes to the player; after
  closing the track menu, focus returns to its button. The arrow-key grid navigation is the
  owner's open question 5 in [m5.md](../plans/m5.md); this principle needs only that Tab order is
  sane until then.
- **Check.** Complete each top task three times: mouse only, keyboard only, phone touch only. Any
  that cannot be done, or where focus is lost or invisible, fails.

- **Targets, sized and placed for the hand and the distance.** Frequent targets are large and
  near where the hand or pointer already is; the primary action is the easiest to reach on a
  phone; destructive actions are never adjacent to frequent ones. Fitts's law: time to a target
  depends on its distance and size [doc] ([NN/g](https://www.nngroup.com/articles/fitts-law/));
  Tognazzini: *"Fitts's Law requires a stop watch test"* [doc]; Wroblewski's *Mobile First*: the
  phone's constraints force the decision of what matters [book]; touch sizes are in [prior]
  (ui-design-practice §2.9). On the phone, the shared player's *Mute* and *Leave* are both within
  thumb reach and far apart, and *End session* is never next to *Pause*. Check: on a phone
  screenshot with the thumb zone marked, a primary action outside it fails, and a destructive
  action within one target width of a frequent one fails.

### 3.9 Watching together

These extend the patterns already recorded in [ui-competitors.md](ui-competitors.md) §4 and placed
in [watch-together.md](../plans/watch-together.md) ("What each person sees"); they turn them into
rules a design is checked against.

#### U22. Everyone always knows who is here, who did what, and what the room is doing

- **Rule.** Every participant sees, at all times: who is present (by the names they chose), whether
  the film is playing, paused or waiting for someone, and each change as a named event in their
  own words. An ending (of the film or of the session) is designed, not a sudden black screen.
- **Why.** Apple, SharePlay: *"Keep people oriented as an activity changes. When one person's action
  changes the activity for everyone, help people understand why… show who's doing what"* [doc].
  Syncplay's named events and Jellyfin's unexplained spinner [prior] (ui-competitors §4). NN/g's
  peak–end rule: experiences are remembered by their most intense moment and their end [doc]
  ([NN/g](https://www.nngroup.com/articles/peak-end-rule/)).
- **In Orpheum.** *"Waiting for Anna, her connection is catching up"* rather than a spinner.
  *"Karl jumped from 12:30 to 41:12"*. When the host ends the film, the viewer sees *"Karl ended
  the film"* and the session's state (*"Karl is choosing what's next"*), not black alone (the plan
  today says screens go black; judgement: the black needs words). When the host ends the session,
  the viewer's page says so and thanks them, not an error.
- **Check.** Freeze any frame of the viewer's screen: can a person who just looked up say who is
  here, what state the film is in and why? Run each event (pause, seek, stall, film change, leave,
  end) and read the viewer's screen: an unexplained change fails.

#### U23. The position is shared; the presentation is personal; control is explicit

- **Rule.** What the group shares (what plays and where) is shown as shared; what is each
  person's (volume, subtitles, audio track, partner volume, quality) is changed without touching
  anyone else, and the controls say which is which. Who may change the shared state is visible.
- **Why.** Apple: *"Let participants customize their experience… Keep these adjustments unique to
  each participant"* [doc]. D188 §4 decided subtitles and audio are per person. On control, Apple
  suggests *"a simple rule, like last change wins"* [doc]; Cut A chose the host alone (D246 §8),
  and D268 §5 keeps the seam for guests' rights later. §4.5 discusses the split.
- **In Orpheum.** The viewer's controls are their own volume, partner volume, (later) subtitles,
  voice and *Leave*; there is no dead play button. When guests are given play and pause later,
  the event line names who pressed it. The host's screen shows *"Anna sees what you play"* while
  hosting (U10).
- **Check.** For each control on the shared player: does it change the group or only this person,
  and does the control's label or placement say so? A control that looks active and does nothing
  for this person fails.

#### U24. Joining is one gesture into the film; leaving and coming back are just as easy

- **Rule.** An invite opens straight into the session; joining asks for a name and one press; a
  person who leaves or drops can come back from the same link without the host; nothing unrelated
  to watching is shown on the way.
- **Why.** Apple: *"Let people join an activity without friction… get them to the shared content
  quickly and avoid showing views unrelated to the activity"*; *"Make it easy to leave and
  rejoin"* [doc]. Invites that need a friendship or an app restart, and re-inviting for every
  episode, are Plex's recorded failures [prior] (ui-competitors §4). The autoplay rule makes the
  join press double as the gesture that unlocks sound [prior].
- **In Orpheum.** Already the design (D267, [watch-together.md](../plans/watch-together.md)). What
  this adds for M5: the join card describes what they are joining (*"Karl invites you to watch
  with him"*, the poster and title when something is on, per Apple's *"Describe activities clearly
  and concisely"* [doc]), and a viewer who closed the tab sees *Rejoin* on reopening, not *Join*.
- **Check.** Time a first-time viewer from opening the link to seeing the film (§5's task 5);
  count screens. Close the tab mid-film and reopen the link: anything but a rejoin fails.

#### U25. Voice is part of the room: its state is unmistakable and its levels are separate

- **Rule.** Whether the microphone is open, muted or failed is shown on screen at all times while
  voice is on; *Mute* is one press from every state of the shared player, including fullscreen;
  the partner's volume and the film's are separate controls.
- **Why.** Geerts et al.: *"when using voice chat, users notice synchronization differences sooner,
  are more annoyed and feel more together than when using text chat"* [doc, abstract]
  ([CHI 2011](https://doi.org/10.1145/1978942.1978986)). Weisz et al.: chat while watching
  *"has a positive influence on social relationships, and people chat despite being distracted"*
  [doc, abstract] ([CHI 2007](https://doi.org/10.1145/1240624.1240756)). One volume for partner
  and film is a recorded complaint (Hearo), and Discord gives each its own [prior]
  (ui-competitors §4). The Media Session's `togglemicrophone` action lets the lock screen carry
  mute [prior].
- **In Orpheum.** Already in [watch-together.md](../plans/watch-together.md) ("Voice"): Talk,
  Mute, Leave voice, the others' volume apart from the film's. What this adds: the mute state
  survives the controls hiding (a small persistent microphone mark), and `m` mutes the film, not
  the microphone, so the two are never confused (judgement: a separate key for the microphone, or
  none, is a question for the owner).
- **Check.** With the player's controls hidden and in fullscreen, can a person tell whether the
  other side hears them? Is mute one action from there?

## 4. Where the practitioners disagree

### 4.1 Modes: abolish them, or signal them

Raskin and Tesler argue for modeless interfaces, with Raskin's *quasimodes* (a state held only
while a key is held) as the alternative [book; doc]. NN/g: modes are *"useful when we have too many
different options… and not enough available types of input"*, and the remedy is strong signalling
[doc]. **Fits here: NN/g's side**, because a television remote and a phone have few inputs, and
because hosting Watch Together is inherently a mode (what you play, others see). Raskin's warning
is kept as U10's two-cue rule and in U23. Judgement.

### 4.2 One way to do a thing, or several

Raskin's *monotony*: one way per atomic task, so it becomes habit [book]. Nielsen's heuristic 7
and Shneiderman's rule 2: shortcuts for experts alongside the visible path [doc]. **Fits here:
both, ordered**: one visible way that is complete, and accelerators that are exact aliases of it
(U15). That honours Raskin's habituation (the action is the same action whichever route) and the
project's [§4](../../principles/04-one-way.md) for code. Judgement.

### 4.3 Confirmations

Raskin: remove warning screens; universal undo makes them unnecessary and habit makes them useless
[book]. NN/g and Apple: keep them for the uncommon and irreversible, and make them specific [doc].
Norman, as NN/g reports, would have a second person confirm the most dangerous [doc]. **Fits
here: undo first (Raskin), confirmation only where undo is impossible (NN/g)** (U8). Watch Together
makes some actions irreversible for someone else (ending the session), which no undo on the
host's side can repair.

### 4.4 Infer, or ask

Victor: interaction is the *"last resort"*; infer from history and environment [doc]. Netflix
personalises even the artwork per member, *"visual 'evidence'"* for why a title suits them [doc]
([TechBlog](https://netflixtechblog.com/artwork-personalization-c589f074ad76)). Tognazzini:
autonomy, the person's decisions are theirs, even inefficient ones [doc]. The project's own §3 and
§15: decisions inspectable and overridable. **Fits here: infer, show, allow override** (U7, U14).
**Personalised artwork does not fit**: in a personal library, a poster is how a person
*recognises* a film they own (U12; Tognazzini's *"stable perceptual cues"*), and nothing is being
sold. Judgement.

### 4.5 Shared control in co-watching

Apple's SharePlay lets everyone control playback, with *last change wins* [doc]; Syncplay and
Discord let anyone pause [prior]. D246 §8 gives control to the host alone in Cut A, with per-link
rights later. **Both are defensible for two people**; host-only removes the conflict case and
suits a guest who did not choose the film, and it costs the guest a basic freedom (*"I need to
pause, the doorbell"*). Judgement: the first test sessions (§5) should record how often the guest
asks the host to pause; that is the measure for D268 §5's rights.

### 4.6 Hide for simplicity, or show for discoverability

Rams (*"as little design as possible"*) and Nielsen's heuristic 8 (*"Every extra unit of
information… competes"*) favour removal [doc]. Tognazzini: *"There is no 'elegance' exception to
discoverability"*; Wroblewski's numbers show hidden features go unused [doc]. **Fits here:
progressive disclosure by measured frequency** (U16), which is the reconciliation NN/g gives.
Plex's 2025 redesign is the field's lesson in getting the split wrong [prior].

### 4.7 Onboarding

NN/g: avoid it [doc]. Apple: avoid it if possible, but if needed make it interactive and optional
[doc]. **Fits here: none** (U20). Watch Together's guest is a first-time user every time, which is
the argument for a short explanation; the join card's one sentence is the whole of it.

### 4.8 How many test users

Nielsen: five users find about 85% of problems, and three rounds of five beat one of fifteen [doc]
([NN/g](https://www.nngroup.com/articles/why-you-only-need-to-test-with-5-users/)). Later critics
(Spool and Schroeder, 2001; Faulkner, 2003; not re-read here) found five too few for large sites
and varied tasks. **Barely relevant here**: there are two users, and §5 adapts to that.

### 4.9 Consistency or improvement

Jakob's law and D108 favour the familiar; Tognazzini's *continuity, not consistency* allows change
that carries habits forward [doc]. The Laws of UX entry on Jakob's law suggests *"empowering users to
continue using a familiar version for a limited time"* [doc]. That is D108's mechanism; no conflict
for this project.

## 5. How to run UX iteration with two users and a staging server

### 5.1 What the practitioners advise

- **Test early, small and often.** Nielsen: *"The best results come from testing no more than 5
  users and running as many small tests as you can afford"*; the first user shows almost a third of
  the problems; fix, then test again, because the fix may not work and may add problems [doc].
  His *discount usability* is simplified testing, paper prototypes and heuristic evaluation, many
  rounds [doc] ([NN/g](https://www.nngroup.com/articles/discount-usability-20-years/)).
- **Hallway testing.** Spolsky: *"grab the next person that passes by in the hallway and force them
  to try to use the code you just wrote"* [doc]
  ([Joel Test](https://www.joelonsoftware.com/2000/08/09/the-joel-test-12-steps-to-better-code/)).
  Krug's *Rocket Surgery Made Easy*: a morning a month, three users, the team watching, fix the
  worst problems first [book].
- **Paper prototypes** before code, for structure: *"one of the fastest and cheapest techniques you
  can employ"*, and the biggest gains *"come from gathering usability data as early as possible"*
  [doc] ([NN/g](https://www.nngroup.com/articles/paper-prototyping/)).
- **Wizard of Oz:** a person plays the system behind a working-looking front, to test behaviour
  that is expensive to build [doc] ([NN/g](https://www.nngroup.com/articles/wizard-of-oz/)).
- **RITE (Rapid Iterative Testing and Evaluation):** change the design *"as soon as a problem is
  identified and a solution is clear… after observing as few as one participant"*; *"once you find
  a problem, solve it as soon as you can, and… make the decision makers part of the research
  team"* (Medlock, Wixon et al., UPA 2002, as summarised in
  [Wikipedia](https://en.wikipedia.org/wiki/RITE_Method); the paper itself was not read).
- **Watch, don't ask.** Tognazzini: *"User-test for discoverability"* and *"User test the visual
  design as thoroughly as the behavioral design"* [doc]. The aesthetic-usability effect hides
  problems behind polish [prior], so tests are of tasks, not impressions.

### 5.2 Adapted to this project

Judgement throughout, built on the advice above.

1. **The two users are different test subjects, and one is not a test subject at all.** The owner
   designed the screens and knows every decision: he cannot find discoverability or wording
   problems in them. The remote person is the real first-time user, and every surface she has not
   used before is tested once only, on first contact. **Save her first contact with each new
   surface for a planned session**, rather than spending it on a build that is still changing.
2. **Hallway testers exist.** D251 records a week of demonstrating Orpheum one to one, mostly on a
   phone, a guest scanning the host's QR code. Each such guest is a first-time Watch Together
   viewer: one task (*join and tell me what's on*), watched, not explained. Three of those per
   round reach Nielsen's minimum for one user group.
3. **A fixed task list, measured each round.** About five tasks, the same every round, each with a
   click target from U4: (1) resume what you were watching; (2) find and play a named film; (3)
   play the next episode of a series; (4) host: start watching a film with someone; (5) viewer:
   from the link to the film playing, with voice. Record success, time, clicks and every hesitation.
   The numbers are thin with two people; their trend across rounds is what matters.
4. **Remote sessions use the product's own voice.** The remote person shares her screen in a
   video call (or the owner watches her side through the host's view) and thinks aloud over
   Watch Together's voice. Five minutes of her narrating the join is worth more than her verdict
   afterwards.
5. **RITE on staging.** When a session shows a problem whose cause and fix are both clear, fix it
   and deploy to staging before the next session, as RITE allows after one participant. The owner
   is the decision maker, already in the room. Unclear problems wait for a second observation.
   Gates are not run on the shared sandbox while a session is live (the staging server shares it).
6. **Paper or static mock-ups for structure, the live build for behaviour.** Questions such as the
   shape of Home or the host's Watch Together screen are cheaper to test as two or three static
   mock-ups on the specimen screen ([ui-architecture-2026-10-05.md](ui-architecture-2026-10-05.md)
   R8) than as built screens. Timing, feedback and sync questions need the live build.
7. **Wizard of Oz for behaviour not yet built.** Before building guest controls (D268 §5) or the
   next-episode countdown in Watch Together, the owner can play the system: the guest says
   *"pause"* aloud and the host pauses, and the session records how often it happens and how it
   felt. That is the measure §4.5 asks for.
8. **A heuristic pass before each session, by an agent, against §3.** NN/g's heuristic evaluation
   uses several evaluators; agents are cheap ones. It finds rule breaches, never what a person
   actually does, so it precedes testing and never replaces it.
9. **Instrument the flows.** Click counts and time to first frame per task, logged on staging, so a
   round's numbers do not depend on someone watching. A problem that needs digging to explain is a
   missing log line.
10. **Keep a findings log per round**: the problem, the evidence (a quote or a count), the
    principle it breaks, the change, and whether the next round confirmed the fix. A fix that the
    next round does not confirm reopens.

## 6. What this means for M5's principles document

- **Candidate rules to carry over:** U1–U25 are written to be checked; the M5 document can adopt
  them as they are, or merge them with the visual principles of
  [ui-design-practice-2026-10-05.md](ui-design-practice-2026-10-05.md) §5.1 under one numbering.
- **Rules a test can hold** (per [§17](../../principles/17-a-rule-nothing-runs.md)):
  U3 (Back and reload restore state) and U21 (keyboard-only completion of the top tasks) are
  browser tests; U4's click counts can be counted by a browser test walking the top tasks; U5's
  100 ms acknowledgement is measurable in the same harness. The rest are review checks.
- **Questions for the owner** that these principles raise:
  1. While hosting, may the host play something only for themselves (U10)?
  2. When the host ends a film without ending the session, what does the viewer see (U22)?
  3. Which key, if any, mutes the microphone (U25)?
  4. Should the first sessions measure how often the guest needs to pause, as the evidence for
     guest controls (§4.5)?

## 7. Where the evidence is weak

- **The books were not re-read.** Norman, Cooper, Krug, Raskin, Sierra and Wroblewski are cited from
  their known content; only their Wikipedia summaries were checked where noted. The chapter-level
  claims (Cooper's *excise* and *perpetual intermediates*, Krug's *trunk test*) are standard
  attributions, not quotations.
- **Netflix's 90-second figure** (Gomez-Uribe and Hunt, *"The Netflix Recommender System"*, ACM
  TMIS 6(4), 2015, [doi](https://doi.org/10.1145/2843948)) is widely quoted as a member losing
  interest *"after perhaps 60 to 90 seconds of choosing, having reviewed 10 to 20 titles"*. **UNVERIFIED
  here**: the ACM page returned 403 and no archived copy was found, so it is cited by reference
  only and not used as the basis of any rule. For a personal library it would argue for U4 and
  against long Home pages, not for recommendation.
- **The co-watching studies are old and small**: Geerts et al. tested 36 participants; Weisz et al.'s
  numbers were not read beyond the abstract. Neither tested a phone carrying voice beside a
  laptop playing.
- **Laws of UX** is a secondary compilation; its laws were used only where a primary source agrees.
- **Nielsen's five-user figure** assumes a group of similar users and a fixed design; the critics
  in §4.8 were not re-read.
- **Most of §3's "In Orpheum" examples and all of §5.2** are this researcher's application of the
  sources, not anyone's tested result.

## 8. Not tried

- Microsoft's Fluent 2 interaction guidance and Material 3's interaction pages were not read for
  this report (Material's site renders only in a browser; the earlier report read its tokens).
  Google TV's design guidelines on developer.android.com render only in a browser and returned no
  readable text.
- No practitioner writing on Teleparty, Discord's Watch Together or Amazon's Watch Party UX was
  found beyond what [ui-competitors.md](ui-competitors.md) and [voice-apps.md](voice-apps.md)
  already hold; no new forum or review sampling was done.
- No usability test of the current client against these principles; §5's first round is that test.
- The original RITE paper (Medlock et al., 2002) and Medlock's 2018 chapter in *Games User
  Research* were not read.
- No search for academic work on remote co-watching after 2011 (for example, studies of
  SharePlay or Teleparty), which may exist.
