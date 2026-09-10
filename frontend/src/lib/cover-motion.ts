/** Shared behavior mirrored in organizer/afisha: one visible, silent cover at a time. */
type Connection = EventTarget & { saveData?: boolean };
type Entry = { video: HTMLVideoElement; source: string; ratio: number; failed: boolean; pending: boolean };
const entries = new Map<HTMLVideoElement, Entry>();
let observer: IntersectionObserver | undefined;
let initialized = false;
let reduced = false;
let saveData = false;
let active: Entry | undefined;

function motionDisabled(): boolean {
  return reduced || saveData;
}

function refresh() {
  if (typeof document === 'undefined') return;
  let winner: Entry | undefined;
  let distance = Infinity;
  if (!motionDisabled() && !document.hidden) {
    for (const entry of entries.values()) {
      if (entry.ratio < 0.35 || entry.failed) continue;
      const rect = entry.video.getBoundingClientRect();
      const next = Math.abs(rect.top + rect.height / 2 - window.innerHeight / 2);
      if (next < distance) { distance = next; winner = entry; }
    }
  }
  active = winner;
  for (const entry of entries.values()) {
    const video = entry.video;
    if (entry !== winner) {
      video.pause();
      video.dataset.playing = 'false';
      // System settings show the still poster and avoid fetching media at all.
      if ((reduced || saveData) && video.hasAttribute('src')) {
        video.removeAttribute('src'); video.load(); delete video.dataset.ready;
      }
      continue;
    }
    if (!video.hasAttribute('src')) video.src = entry.source;
    if (entry.pending || !video.paused) continue;
    video.muted = true;
    entry.pending = true;
    let aborted = false;
    void video.play().catch((error: unknown) => {
      if (entries.get(video) !== entry) return;
      if (error instanceof DOMException && error.name === 'AbortError') { aborted = true; return; }
      entry.failed = true;
      video.dataset.failed = 'true';
      delete video.dataset.ready;
      refresh();
    }).finally(() => {
      entry.pending = false;
      if (aborted && entries.get(video) === entry && active === entry) queueMicrotask(refresh);
    });
  }
}

function initialize() {
  if (initialized || typeof window === 'undefined') return;
  initialized = true;
  // The old preview had a pause button. Removing it must not strand returning
  // visitors on a persisted pause they can no longer undo.
  try { localStorage.removeItem('vshage.cover-motion-paused'); } catch { /* private mode */ }
  const media = window.matchMedia('(prefers-reduced-motion: reduce)');
  const connection = (navigator as Navigator & { connection?: Connection }).connection;
  const preferencesChanged = () => {
    reduced = media.matches;
    saveData = connection?.saveData === true;
    refresh();
  };
  preferencesChanged();
  media.addEventListener('change', preferencesChanged);
  connection?.addEventListener('change', preferencesChanged);
  document.addEventListener('visibilitychange', refresh);
  window.addEventListener('resize', refresh);
  if (typeof IntersectionObserver !== 'undefined') {
    observer = new IntersectionObserver(changes => {
      for (const change of changes) {
        const entry = entries.get(change.target as HTMLVideoElement);
        if (entry) entry.ratio = change.isIntersecting ? change.intersectionRatio : 0;
      }
      refresh();
    }, { threshold: [0, 0.2, 0.35, 0.5, 0.75, 1] });
  }
}

export function registerCoverVideo(video: HTMLVideoElement, source: string): () => void {
  initialize();
  const entry: Entry = { video, source, ratio: 0, failed: false, pending: false };
  entries.set(video, entry);
  delete video.dataset.ready; delete video.dataset.failed; video.dataset.playing = 'false';
  video.muted = true; video.loop = true; video.playsInline = true;
  video.controls = false; video.preload = 'none';
  const playing = () => {
    if (active !== entry || motionDisabled() || document.hidden) { video.pause(); return; }
    video.dataset.ready = 'true'; video.dataset.playing = 'true';
  };
  const paused = () => { video.dataset.playing = 'false'; };
  const failed = () => {
    entry.failed = true; video.dataset.failed = 'true'; delete video.dataset.ready; refresh();
  };
  video.addEventListener('playing', playing);
  video.addEventListener('pause', paused);
  video.addEventListener('error', failed);
  observer?.observe(video);
  refresh();
  return () => {
    observer?.unobserve(video); entries.delete(video);
    video.removeEventListener('playing', playing); video.removeEventListener('pause', paused); video.removeEventListener('error', failed);
    video.pause(); video.removeAttribute('src'); video.load();
    refresh();
  };
}
