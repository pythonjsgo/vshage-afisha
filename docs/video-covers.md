# Video covers — DEV iteration

A cover is a still poster plus an optional silent MP4 loop. It has no native
player controls, fullscreen button, timeline or audio. A small motion toggle
pauses/resumes cover animations. At most one visible cover plays per page;
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
  pause offscreen, global motion toggle, reduced-motion/data-saver fallback,
  broken-video fallback and normal card navigation. All browsers headless.

Deploy to DEV only for this request. Apply 024 before organizer API, then
Afisha backend and both frontends. Do not put an MP4 in an existing image
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

Headless Chromium checks passed for desktop, mobile viewport, actual time
advancement, controls disabled, pause/resume, offscreen/resume, a failed MP4,
reduced motion and data saver (both zero MP4 requests), listing motion button
and pointer navigation. This is browser emulation, not native iOS validation.

Reproduce the read-only browser checks from `frontend/`:

```sh
npx playwright install chromium --only-shell
VSHAGE_VIDEO_EVENT_URL=https://afisha.dev.vshage.app/04364617-8ef8-477d-b578-76407f95c777 \
  node scripts/verify-video-cover.mjs
```

The script permits only DEV/local hosts and submits no registration. A
temporary DOM spacer verifies offscreen behavior even for a short page.
