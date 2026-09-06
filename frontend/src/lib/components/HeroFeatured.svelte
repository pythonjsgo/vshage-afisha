<script lang="ts">
  import type { PublicEvent } from '$lib/types';
  import { formatWhen } from '$lib/dateFormat';
  import { onMount } from 'svelte';
  import GlitchText from './GlitchText.svelte';

  let { events }: { events: PublicEvent[] } = $props();
  let idx = $state(0);
  let timer: ReturnType<typeof setInterval> | null = null;

  onMount(() => {
    if (events.length <= 1) return;
    timer = setInterval(() => { idx = (idx + 1) % events.length; }, 6000);
    return () => { if (timer) clearInterval(timer); };
  });

  const active = $derived(events[idx]);
</script>

{#if active}
  <!-- Подпись «когда» здесь та же, что на карточках ниже, и по той же причине.
       Прежде печаталась дата НАЧАЛА, а у закреплённой идущей программы в этом
       поле стоит дата поста: выставка, открывшаяся в июне, объявляла себя
       сегодняшней — тем же шрифтом в 96 пикселей, каким мы зовём человека
       прийти. Заодно ушло выдуманное «00:00» у событий без известного времени:
       formatEventDateLong звалась здесь без признака start_time_known. -->
  {@const when = formatWhen(active)}
  <a class="hero" href={active.webreg_slug ? `/e/${active.webreg_slug}` : `/${active.id}`}
     style={active.photo_url ? `background-image: url(${active.photo_url})` : ''}>
    <div class="overlay"></div>
    <div class="content">
      <div class="kicker">FEATURED · {String(idx + 1).padStart(2, '0')} / {String(events.length).padStart(2, '0')}</div>
      <!-- Заголовок закреплённого события — НЕ <h1>. Главный заголовок
           страницы теперь принадлежит городу или разделу («Концерты в
           Москве»), и второй h1 внутри карусели размывает тему страницы для
           поисковика: он читает их как два конкурирующих утверждения о том,
           про что страница. Класс и вид не меняются — селектор .title. -->
      <p class="title">
        <GlitchText text={active.title} />
      </p>
      <div class="meta">
        <span class="when" class:urgent={when.urgent}>{when.text}</span>
        {#if active.location}
          <span> · {active.location}</span>
        {/if}
        {#if active.attendee_count > 0}
          <span> · {active.attendee_count} ИДУТ</span>
        {/if}
      </div>
    </div>
    <div class="pager">
      {#each events as _, i}
        <div class:on={i === idx}></div>
      {/each}
    </div>
  </a>
{/if}

<style>
  .hero {
    position: relative;
    display: block;
    min-height: 380px;
    padding: var(--sp-5);
    background: linear-gradient(135deg, #200428 0%, #0a0a0a 100%);
    background-size: cover;
    background-position: center;
    border: 1px solid var(--border);
    overflow: hidden;
    text-decoration: none;
    color: var(--fg);
  }
  .overlay {
    position: absolute; inset: 0;
    background: linear-gradient(180deg, transparent 40%, rgba(0,0,0,0.8) 100%);
  }
  .content { position: relative; z-index: 1; display: flex; flex-direction: column; gap: var(--sp-3); min-height: 320px; justify-content: flex-end; }
  .kicker { font-size: 10px; letter-spacing: 2px; color: var(--accent-green); }
  .title {
    font-family: var(--font-display);
    font-weight: 400;
    font-size: clamp(40px, 9vw, 96px);
    line-height: 0.9;
    text-transform: uppercase;
    letter-spacing: -2px;
  }
  .meta { font-size: 11px; letter-spacing: 1px; color: var(--mute); }
  /* «ПОСЛЕДНИЙ ДЕНЬ» и «ОСТАЛОСЬ 2 ДНЯ» вынимаются из приглушённой строки
     розовым — тем же, что на карточках. Закреплённая программа, которая
     закрывается сегодня, — единственное место доски, где от прочтения этой
     строки зависит, попадёт человек или нет. */
  .when.urgent { color: var(--accent-pink); }
  .pager { position: absolute; bottom: var(--sp-3); left: var(--sp-5); right: var(--sp-5); display: flex; gap: var(--sp-1); z-index: 2; }
  .pager div { flex: 1; height: 2px; background: var(--border); }
  .pager div.on { background: var(--accent-pink); }
</style>
