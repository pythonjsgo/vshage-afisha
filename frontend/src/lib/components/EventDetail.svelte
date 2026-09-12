<script lang="ts">
  import { onMount } from 'svelte';
  import type { PublicEvent } from '$lib/types';
  import { formatEventDateLong, formatEndDate, formatWhen } from '$lib/dateFormat';
  import { categoryLabel } from '$lib/taxonomy';
  import MetaPill from './MetaPill.svelte';
  import GlitchText from './GlitchText.svelte';
  import ShareSheet from './ShareSheet.svelte';
  import AppCTA from './AppCTA.svelte';
  import LiveCounter from './LiveCounter.svelte';
  import EventGallery from './EventGallery.svelte';
  import EventCover from './EventCover.svelte';
  import { eventAction, eventPrice, eventSource, httpURL } from '$lib/event-actions';
  import { documentEventLocale, eventCopy, type EventLocale, type EventCopyKey } from '$lib/event-copy';

  let { event, origin }: { event: PublicEvent; origin: string } = $props();
  const url = $derived(`${origin}/${event.id}`);
  const cancelled = $derived(event.status === 'cancelled');
  let locale = $state<EventLocale>('ru');
  onMount(() => { locale = documentEventLocale(); });
  const t = (key: EventCopyKey) => eventCopy(key, locale);
  const action = $derived(eventAction(event));
  const price = $derived(eventPrice(event, locale));
  const source = $derived(eventSource(event));
  const place = $derived(event.venue_name || event.location || event.address);
  // Подпись рубрики — ТОЛЬКО через словарь. Здесь печатался сам код, и с 07.09,
  // когда категория поехала наружу из tgevents, на карточке появилось сырое
  // «CAMPUS» / «THEATRE_CINEMA». Кода вне словаря быть не должно, но если он
  // приедет — чип не рисуется вовсе: пусто честнее идентификатора.
  const category = $derived(categoryLabel(event.category)?.toUpperCase() ?? '');
  /**
   * Подпись «когда» у длящейся программы собирает formatWhen: «ИДЁТ ДО 20 СЕН»,
   * «ПОСЛЕДНИЙ ДЕНЬ», «28 СЕН — 3 ОКТ», «ИДЁТ ПОСТОЯННО». Дата начала у идущей
   * НЕ печатается: в базе там чаще дата поста, а не открытия (у биеннале,
   * работающей с марта, стоит 5 сентября).
   *
   * Точечное событие остаётся на прежней длинной подписи. Относительное
   * «СЕГОДНЯ В 19:00», которое отдаёт formatWhen, годится карточке в ленте, но
   * в строке «КОГДА» человек ищет именно календарную дату — её он переписывает
   * себе в календарь.
   */
  const when = $derived(event.multiday ? formatWhen(event).text : '');
  // Импортированное студсобытие. Признак ЯВНЫЙ (source), а не «есть
  // source_url»: то поле nullable, и карточка без него превратилась бы в
  // «наше» событие с нашей кнопкой записи на чужое мероприятие.
  const imported = $derived(event.source === 'tg');
  const accessLabel = $derived(
    event.access_level === 'university' ? t('students')
    : event.access_level === 'invite' ? t('invite')
    : ''
  );
</script>

<article class="detail" class:cancelled>
  <header class="cover">
    <EventCover poster={event.photo_url} video={event.cover_video_url} eager />
    <div class="overlay"></div>
    <nav class="back-nav"><a href="/">← {t('afisha')}</a></nav>
    <div class="hd">
      <h1><GlitchText text={event.title} /></h1>
      <div class="pills">
        {#if cancelled}<MetaPill text={t('cancelled')} variant="warning" />{/if}
        {#if category}<MetaPill text={category} />{/if}
        {#if event.max_attendees}<MetaPill text={`LIMIT ${event.max_attendees}`} />{/if}
        {#if accessLabel}<MetaPill text={accessLabel} />{/if}
      </div>
    </div>
  </header>

  {#if event.photos && event.photos.length > 0}
    <EventGallery photos={event.photos} />
  {/if}

  <section class="info">
    <div class="row"><div class="k">{t('when')}</div><div class="v">
      {#if when}
        {when}
      {:else}
        {formatEventDateLong(event.start_time, event.start_time_known !== false)}
        {#if event.end_time}<br />{formatEndDate(event.end_time)}{/if}
      {/if}
    </div></div>
    {#if event.end_date}
      <div class="row"><div class="k">{t('ends')}</div><div class="v">{formatEventDateLong(event.end_date, event.end_date.includes('T'))}</div></div>
    {/if}
    {#if event.performers?.length}
      <div class="row"><div class="k">{t('performers')}</div><div class="v">{event.performers.map(p => p.name).join(', ')}</div></div>
    {/if}
    {#if event.offers_valid_from}
      <div class="row"><div class="k">{t('salesStart')}</div><div class="v">{formatEventDateLong(event.offers_valid_from)}</div></div>
    {/if}
    {#if place}
      <div class="row"><div class="k">{t('where')}</div><div class="v">{place}
        {#if event.address && event.address !== place}<span class="address">{event.address}</span>{/if}
      </div></div>
    {/if}
    {#if event.organizer_name}
      <div class="row organizer-row">
        <div class="k">{t('organizer')}</div>
        <div class="v organizer">
          {#if event.organizer_photo}
            <img class="avatar" src={event.organizer_photo} alt={event.organizer_name} />
          {:else}
            <span class="avatar ph">{event.organizer_name.charAt(0).toUpperCase()}</span>
          {/if}
          {#if event.organizer_url && httpURL(event.organizer_url)}
            <a href={event.organizer_url} target="_blank" rel="noopener noreferrer">{event.organizer_name}</a>
          {:else}<span>{event.organizer_name}</span>{/if}
        </div>
      </div>
    {/if}
    {#if event.attendee_count > 0}
      <div class="row"><div class="k">{t('who')}</div><div class="v"><LiveCounter value={event.attendee_count} label={t('going')} /></div></div>
    {/if}
  </section>

  {#if action || price}
    <section class="register-sec" aria-label={t('participation')}>
      <div class="booking">
        {#if price}<div class="booking-price"><span>{t('price')}</span><strong>{price}</strong></div>{/if}
        {#if action}<div class="booking-action">
          <a class="register-btn" href={action.href} target={action.external ? '_blank' : undefined}
            rel={action.external ? 'noreferrer' : undefined}>{t(action.label)} {action.external ? '↗' : '→'}</a>
          {#if source && !(action.label === 'details' && action.href === source.href)}
            <a class="source-link" href={source.href} target="_blank" rel="noreferrer">
              {t(event.source_url ? 'eventSource' : 'organizerSite')} · {source.host}
            </a>
          {/if}
        </div>{/if}
      </div>
    </section>
  {/if}

  {#if event.description}
    <section class="desc">{event.description}</section>
  {/if}

  <section class="share-sec"><ShareSheet {url} title={event.title} /></section>

  {#if !imported}
    <section class="cta-sec">
      <AppCTA eventId={event.id} />
    </section>
  {/if}
</article>

<style>
  .detail { max-width: 1200px; margin: 0 auto; }
  .detail.cancelled { opacity: 0.65; filter: grayscale(1); }
  .cover {
    position: relative;
    min-height: 420px;
    padding: clamp(20px, 3vw, 40px);
    background: linear-gradient(135deg, var(--accent-pink) 0%, #1a0014 100%);
    background-size: cover; background-position: center;
    display: flex; flex-direction: column; justify-content: space-between;
  }
  .overlay { position: absolute; inset: 0; background: linear-gradient(180deg, rgba(0,0,0,0.06) 20%, rgba(0,0,0,0.9) 100%); }
  .back-nav, .hd { position: relative; z-index: 1; }
  .back-nav a { color: var(--fg); font-size: 11px; letter-spacing: 1px; text-transform: uppercase; }
  .hd h1 { max-width: 900px; font-family: var(--font-display); font-size: clamp(36px, 5.8vw, 64px); line-height: 1; text-transform: uppercase; font-weight: 400; letter-spacing: -1px; }
  .pills { display: flex; gap: var(--sp-1); flex-wrap: wrap; margin-top: var(--sp-3); }
  .info { padding: var(--sp-4); display: grid; grid-template-columns: 1fr 1fr; column-gap: var(--sp-5); }
  .row { display: flex; justify-content: space-between; padding: var(--sp-3) 0; border-bottom: 1px solid var(--border); font-size: 12px; }
  .row .k { color: var(--mute); letter-spacing: 1.5px; font-size: 10px; text-transform: uppercase; flex-shrink: 0; }
  .row { gap: 20px; }
  .row .v { text-align: right; }
  .address { display: block; margin-top: 6px; color: var(--mute); font-size: 11px; }
  .organizer { display: flex; align-items: center; gap: var(--sp-2); }
  .avatar {
    width: 24px; height: 24px; border-radius: 50%; object-fit: cover;
    background: var(--bg-elev); border: 1px solid var(--border);
    display: flex; align-items: center; justify-content: center;
    font-size: 10px; font-weight: 700; color: var(--accent-pink);
  }
  .desc { max-width: 82ch; padding: var(--sp-4); font-size: 15px; line-height: 1.75; color: #ddd; white-space: pre-line; }
  .share-sec, .register-sec, .cta-sec { padding: var(--sp-3) var(--sp-4); }
  .share-sec { border-top: 1px solid var(--border); }
  .booking { display: flex; align-items: center; justify-content: space-between; gap: 28px; padding: 24px; border: 1px solid var(--border); background: var(--bg-elev); }
  .booking-price { display: flex; flex-direction: column; gap: 6px; }
  .booking-price span { color: var(--mute); font-size: 11px; }
  .booking-price strong { font-size: 24px; font-weight: 500; }
  .booking-action { min-width: min(100%, 320px); }
  .source-link { display: block; margin-top: 10px; text-align: center; font-size: 11px; color: var(--mute); overflow-wrap: anywhere; }
  .source-link:hover { color: var(--fg); }
  .register-btn {
    display: block;
    width: 100%;
    padding: var(--sp-3);
    background: var(--accent-green);
    color: #000;
    border: none;
    font-family: var(--font-mono);
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 1.5px;
    text-align: center;
    transition: background var(--dur-fast);
  }
  .register-btn:hover { background: var(--accent-pink); }
  .register-btn:focus-visible, .source-link:focus-visible { outline: 2px solid var(--accent-green); outline-offset: 4px; }
  @media (max-width: 640px) {
    .cover { min-height: 380px; }
    .info { grid-template-columns: 1fr; }
    .booking { align-items: stretch; flex-direction: column; gap: 18px; padding: 20px; }
    .booking-price strong { font-size: 22px; }
    .booking-action { min-width: 0; }
    .desc { font-size: 14px; }
  }
</style>
