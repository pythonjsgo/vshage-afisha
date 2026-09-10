# Video covers

A cover is a still poster plus an optional silent MP4 loop. It has no native
player controls, play/pause button, fullscreen button, timeline or audio.
At most one visible cover plays per page;
hidden tabs and offscreen covers pause. Reduced motion and data saver keep
the poster without loading the video. Playback errors also keep the poster.

## Contract and compatibility

- `events.photo_url` / organizer `image_url` / afisha `photo_url` always hold
  a real image. Existing app builds and OG generation keep reading it.
- Optional `organizer_event_details.cover_video_url` is added by organizer
  migration 024. No existing row changes meaning. Afisha reads it through
  `to_jsonb(d)` so an older DB without that key continues serving images.
- Create and Update (including MCP) accept `cover_video_url`. A non-empty
  video requires an image poster. Omission preserves the video; `""` clears
  it while retaining the poster. Old clients ignore the additive field.
- Cover edits preserve existing price, currency, external registration,
  other details and custom fields. `registration_deadline: null` clears the
  deadline explicitly; omission leaves it untouched.

## Upload

`POST /api/organizer/uploads/event-video` with the same account Bearer and a
multipart `file`. MP4 or modern MOV, up to 50 MiB. The server verifies an ISO
BMFF container and a video track, then generates at most 20 seconds of H.264
YUV420p, 24 fps, at most 1280×720, no audio, bounded bitrate and fast-start.
A JPEG from the middle of the clip is the poster. Originals stay temporary.
One media job per API process, 90-second deadline, two codec threads. A busy
worker answers 503 with Retry-After. Storage uses the existing object store
or persistent upload directory. ffmpeg/ffprobe are included in the API image.

Response: `url` (JPEG, compatible with image uploads), `cover_video_url`,
`duration`, `width`, `height`, `size`. React upload/create/edit uses both URLs.

## Web surfaces

Afisha card grid, featured hero, detail header and registration header use
`EventCover.svelte`; organizer uses `EventCoverMedia.tsx` in cards, detail and
upload preview. `cover-motion.ts` is intentionally mirrored across repos:
keep its playback policy synchronized. Native app playback is a separate
iteration; this change only provides its safe poster fallback.

## Validation

- Real ffmpeg test: audio removed, bounded duration, JPEG dimensions.
- Real HTTP upload: poster + clip, MP4 Range request returns 206.
- PostgreSQL/MCP test: cover-only edits preserve paid/external registration;
  old DTO reads an image; omission and explicit clearing differ.
- Browser checks must verify time advancement, no native controls/audio,
  visible changing frames, opacity, pause offscreen, no buttons, reduced-motion/data-saver fallback,
  broken-video fallback and normal card navigation. All browsers headless.

The user approved production rollout on 2026-09-10 after the DEV preview.
Apply 024 before organizer API, then Afisha backend and both frontends. Do not put an MP4 in an existing image
field. Rollback images can ignore the nullable column and keep the posters.

Browser policy references: [WebKit inline/autoplay policy](https://webkit.org/blog/6784/new-video-policies-for-ios/),
[MDN autoplay](https://developer.mozilla.org/en-US/docs/Web/Media/Guides/Autoplay),
[reduced motion](https://developer.mozilla.org/en-US/docs/Web/CSS/@media/prefers-reduced-motion).

## DEV preview, 2026-09-10

[INVEST PADEL](https://afisha.dev.vshage.app/04364617-8ef8-477d-b578-76407f95c777)
uses the supplied 15.5-second clip, normalized to 848×464 H.264/YUV420p,
1,350,858 bytes, one video stream and no audio. The JPEG is a separate asset;
Range GET returns 206. Price is 5,000 RUB and registration links to Bogdan.
The DEV organizer is isolated; the event's venue was not supplied.

The first Chromium check verified time advancement but missed transparent
video; it was not proof of visible playback. The corrected headless Chromium
and WebKit checks verify opacity and changing rendered frames on desktop,
mobile and listing views, no controls, offscreen/resume, failed MP4 fallback,
reduced motion and data saver (both zero MP4 requests), and pointer navigation.
This is browser emulation, not native iOS device validation.

Reproduce the read-only browser checks from `frontend/`:

```sh
npx playwright install chromium --only-shell
npx playwright install webkit
VSHAGE_VIDEO_EVENT_URL=https://afisha.dev.vshage.app/04364617-8ef8-477d-b578-76407f95c777 \
  node scripts/verify-video-cover.mjs
```

The script permits only DEV/local hosts and submits no registration. A
temporary DOM spacer verifies offscreen behavior even for a short page.

## Visible playback fix and legacy feed contract

The first Svelte build removed `video[data-ready='true']` as an unused
selector: the action sets that attribute outside the template. The video
clock advanced under opacity0. `video:global([data-ready='true'])` retains the
scoped runtime selector. The two warnings from EventCover were new warnings,
not part of the pre-existing baseline. The corrected Svelte check has11
remaining warnings in other files and none in EventCover.

The user requested removal of every play/pause button. Both web covers now
have no controls, and the obsolete `vshage.cover-motion-paused` preference is
removed on initialization so returning visitors cannot get stuck on pause.
System reduced motion and data saver still show the poster.

Browser verification now asserts computed opacity1 as well as an advancing
clock and different rendered frame screenshots. Global scanlines and title
overlays are suppressed only during frame comparison, so those animations
cannot masquerade as video motion. The known-bad DEV build was the negative
control: clock advancing, opacity0, identical rendered frames. Run the same
script with `VSHAGE_VIDEO_BROWSER=webkit` for the second browser engine.

The actual core API response at
`https://api.dev.vshage.app/public/events/app/04364617-8ef8-477d-b578-76407f95c777`
keeps the JPEG in `cover_url` (sourced from events.photo_url), not the MP4.
The actual pre-video Swift EventCard model decoded this payload, and ImageIO
decoded its downloaded poster at848×464. Native EventCoverImage reads that
same coverURL through its image loader. No native or core API change is
needed for this fallback; a future video-capable feed adds an optional
video URL while keeping cover_url as the poster. Build120 predates these
feed cards entirely; this check does not claim to render a new feed on it.
