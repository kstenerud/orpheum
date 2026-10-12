> **ABOUTME:** Why an iPhone played a film silently with its ring/silent switch on, in Watch Together
> and alone, though the page said it played with sound: WebKit's audio-session rules for a video,
> Web Audio and WebRTC, read in iOS 18.5's source, and the WebKit bug that matches. Ends in a device
> test and what the page would change if it is confirmed.

# The iPhone's silent switch and a film's sound

Researched **2026-10-12**, after the owner's VQ30 runs: an iPhone with its silent switch on played the
film with no sound, in a Watch Together session and alone, while the core's log said the element was
unmuted, at volume 1, playing, and `navigator.audioSession.type` read "playback"; with the switch off
it had sound. Sources are cached in `/yoloai/cache/silent-switch-research/` (WebKit files from `main`
and `safari-7621-branch`, which is iOS 18.5's WebKit, and the bug threads). **[V]** is read in source
or a bug report, **[I]** inferred.

## Findings

1. **A video fed by MSE or ManagedMediaSource is typed as any other.** An element that has video, is
   not muted, has volume above 0 and an audio track (for MSE, any active SourceBuffer with audio) is
   "video with audio", which makes the page's category MediaPlayback, not muted by the switch. There is
   no MSE branch. (`HTMLMediaElement::mediaType()`, `canProduceAudio()`; `MediaSourcePrivate::hasAudio()`;
   `MediaSessionManagerCocoa::updateSessionState()`, safari-7621-branch.) **[V]**
2. **A video started muted and unmuted later is re-typed:** unmuting calls `canProduceAudioChanged()`,
   which activates the session and updates its category to MediaPlayback. A field report agrees: a
   `<video playsinline>` started muted and unmuted is audible with the switch on
   ([bug 323022](https://bugs.webkit.org/show_bug.cgi?id=323022)). **[V]**
3. **How WebKit chooses the category:** an `audioSession.type` the page sets wins; else capture
   (an open microphone) gives PlayAndRecord; else audible video, then audible audio, gives
   MediaPlayback; else Web Audio alone gives AmbientSound, which the switch mutes. Receiving WebRTC
   audio does not force PlayAndRecord; only capture does. "playback" maps to MediaPlayback
   ([DOMAudioSession.cpp](https://github.com/WebKit/WebKit/blob/main/Source/WebCore/Modules/audiosession/DOMAudioSession.cpp)).
   **[V]** So neither Watch Together's voice nor the film's feed explains the silence by these rules.
4. **The bug that matches:** [bug 323104](https://bugs.webkit.org/show_bug.cgi?id=323104), iOS 18.7, a
   ManagedMediaSource player: nothing in the tab is audible while every API says playback; it begins at a
   MediaSource's teardown; it survives reloads and back/forward in the same tab; turning the switch to
   ring brings the sound back at once; quitting Safari or opening the address afresh clears it; and
   setting `navigator.audioSession.type` to "ambient" and then, a tick later, "playback" heals it, where
   setting "playback" again alone does nothing. The reporter's cause: after the GPU process restarts,
   the web process keeps its cached category and never sends it again, and the GPU process falls back
   to none, which iOS treats as ambient. **[V]** In iOS 18.5's source `RemoteAudioSession::setCategory`
   skips sending a category equal to its cached one, the cache is not cleared when the GPU process's
   connection closes, and reconnecting does not send it again
   ([RemoteAudioSession.cpp, safari-7621-branch](https://github.com/WebKit/WebKit/blob/safari-7621-branch/Source/WebKit/WebProcess/GPU/media/RemoteAudioSession.cpp)). **[V]**
5. **It fits what was seen:** silence only with the switch on, "playback" reported, the element
   unmuted and playing; iOS often ends Safari's GPU process when Safari is in the background, as it is
   between tests; and a "film alone" test in the same tab would inherit the stuck state, which is why
   leaving out WebRTC and Web Audio changed nothing. **[I]**
6. **Not checked:** how YouTube's or Netflix's web players behave with the switch on.

## A device test

1. Quit Safari from the app switcher, set the switch to silent, type the film's address into a new
   tab, play, and tap for sound. Audible: the stuck state is the cause.
2. Put Safari in the background for half a minute and come back. Silent again: the stuck state
   follows a GPU process's restart.

**Confirmed 2026-10-12 by the owner:** after Safari was quit, the iPhone played the film with the
switch on (*"After restarting,it played with the silence button turned on"*).

## What the page does (built 2026-10-12, `web/packages/sdk/src/page-audio.ts`)

- Set `navigator.audioSession.type = "playback"` from the page's start, before the film or any audio
  context plays, where the browser has it.
- Set it to "ambient" and, a tick later, "playback" again at each tap for sound, each return of the page
  to visible, and after the video's `emptied` (a media source torn down): cheap, and the heal bug 323104
  found.
- Report the result on bug 323104.
- Not worth doing for this: native HLS instead of ManagedMediaSource (the same category path), or
  voice moved off Web Audio (no difference while "playback" is set).
