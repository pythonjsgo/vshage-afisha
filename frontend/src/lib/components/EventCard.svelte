<script lang="ts">
  import type { PublicEvent } from '$lib/types';
  import { formatWhen } from '$lib/dateFormat';
  import MetaPill from './MetaPill.svelte';
  import { categoryLabel } from '$lib/taxonomy';

  let { event }: { event: PublicEvent } = $props();
  // События веб-регистрации адресуются слагом, а не uuid. Ведём на карточку
  // афиши, а не на страницу регистрации: из ленты человек идёт смотреть, что
  // это за событие, и уже оттуда — записываться (директива 17.08).
  const href = $derived(event.webreg_slug ? `/${event.webreg_slug}` : `/${event.id}`);
  const cancelled = $derived(event.status === 'cancelled');
  // Подпись рубрики — ТОЛЬКО через словарь. Печатать `event.category` как
  // есть значило бы вывести на экран код (`CAMPUS`, `THEATRE_CINEMA`) — ровно
  // та ловушка, что уже была с `feed.category.campus` в ленте приложения.
  // Кода вне словаря не бывает (бэкенд его не принимает), но если он всё же
  // приедет — чип не рисуется вовсе: пусто честнее идентификатора.
  const category = $derived(categoryLabel(event.category)?.toUpperCase() ?? '');
  const imported = $derived(event.source === 'tg');
  // Подпись «когда» целиком собирает formatWhen: у точечного события это дата
  // со временем, у идущей программы — «до когда», у периода впереди — диапазон.
  // Двух отдельных кусков (дата начала + «до …») здесь больше нет: у идущей
  // выставки датой начала стояла дата ПОСТА, и карточка биеннале, работающей с
  // марта, честно писала «5 СЕН · до 30 НОЯ» — то есть врала о начале.
  const when = $derived(formatWhen(event));
  const restrictedAccess = $derived(
    event.access_level === 'university' || event.access_level === 'invite'
  );
  // Уровень доступа. С 06.09 у импортированных карточек есть и рубрика, но
  // приоритет остаётся за доступом, когда он ОГРАНИЧЕН: человеку до клика
  // важнее знать, пустят ли его, чем что это «кампус». (Прежде здесь стояло
  // «у импортированных рубрики нет» — это перестало быть правдой в тот
  // момент, когда категория поехала наружу из tgevents.)
  const accessLabel = $derived(
    event.access_level === 'university' ? 'ДЛЯ СТУДЕНТОВ'
    : event.access_level === 'invite' ? 'ПО ПРИГЛАШЕНИЮ'
    : event.access_level === 'open' ? 'ОТКРЫТЫЙ ВХОД'
    : ''
  );
</script>

<a {href} class="card" class:cancelled>
  <div
    class="photo"
    class:no-photo={!event.photo_url}
    style={event.photo_url ? `background-image: url(${event.photo_url})` : ''}
  >
    {#if !event.photo_url}
      <div class="ph-pattern"></div>
    {/if}
  </div>
  <div class="body">
    <div class="top">
      {#if cancelled}
        <MetaPill text="Отменено" variant="warning" />
      {:else if restrictedAccess}
        <MetaPill text={accessLabel} />
      {:else if category}
        <MetaPill text={category} />
      {:else if accessLabel}
        <MetaPill text={accessLabel} />
      {/if}
    </div>
    <h3>{event.title}</h3>
    <div class="meta">
      <span class="date" class:urgent={when.urgent}>{when.text}</span>
      {#if event.attendee_count > 0}
        <span class="att">· {event.attendee_count} идут</span>
      {/if}
    </div>
    {#if event.organizer_name}
      <div class="org">by {event.organizer_name}</div>
    {/if}
  </div>
</a>

<style>
  .card {
    display: block;
    background: var(--bg-elev);
    border: 1px solid var(--border);
    color: var(--fg);
    transition: transform var(--dur-fast) var(--ease-out),
                box-shadow var(--dur-fast) var(--ease-out),
                border-color var(--dur-fast) var(--ease-out);
  }
  .card:hover {
    transform: translateY(-2px);
    border-color: var(--accent-pink);
    box-shadow: 0 0 0 1px var(--accent-pink), 0 0 24px rgba(255, 0, 204, 0.25);
  }
  .cancelled { opacity: 0.5; filter: grayscale(1); }
  .photo {
    aspect-ratio: 4 / 3;
    background-color: var(--border);
    background-size: cover;
    background-position: center;
    position: relative;
  }
  /* Без обложки карточка не должна зиять пустотой на пол-экрана —
     это ровно то, что читается как «пустое меро». Сжимаем заглушку до
     узкой полосы, содержимое карточки становится главным. */
  .photo.no-photo {
    aspect-ratio: auto;
    height: 6px;
  }
  .ph-pattern {
    position: absolute; inset: 0;
    background:
      linear-gradient(45deg,
        var(--border) 25%, transparent 25%,
        transparent 50%, var(--border) 50%,
        var(--border) 75%, transparent 75%);
    background-size: 12px 12px;
    opacity: 0.4;
  }
  .body { padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-2); }
  h3 {
    font-family: var(--font-display);
    font-size: 18px;
    line-height: 1;
    text-transform: uppercase;
    font-weight: 400;
    letter-spacing: -0.5px;
  }
  .meta { font-size: 11px; color: var(--mute); }
  .date { color: var(--accent-green); }
  /* «ПОСЛЕДНИЙ ДЕНЬ» и «ОСТАЛОСЬ 2 ДНЯ» — единственные подписи, где от чтения
     зависит, успеет человек или нет. Розовым в этой теме подсвечено только
     то, что требует действия сейчас; сделать таким же обычную дату значило бы
     обесценить сам сигнал. */
  .date.urgent { color: var(--accent-pink); }
  .org {
    font-size: 10px;
    color: var(--mute);
    opacity: 0.6;
    letter-spacing: 0.5px;
    margin-top: 2px;
  }
</style>
