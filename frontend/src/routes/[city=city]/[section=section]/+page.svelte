<script lang="ts">
  import Listing from '../Listing.svelte';
  let { data } = $props();
</script>

<!-- Ключ на город+раздел+полосу: /msk/concert → /msk/party переиспользует
     ОДИН и тот же компонент страницы, и без ключа карточки концертов остались
     бы в состоянии рядом с вечеринками. Полоса меняется параметром адреса —
     маршрут тот же самый, значит и её надо назвать в ключе. -->
{#key `${data.city.slug}/${data.section?.slug ?? ''}?${data.kind ?? ''}`}
  <Listing {...data} />
{/key}
