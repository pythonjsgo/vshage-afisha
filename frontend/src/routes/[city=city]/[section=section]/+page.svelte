<script lang="ts">
  import Listing from '../Listing.svelte';
  let { data } = $props();
</script>

<!-- Ключ на город+раздел: /msk/concert → /msk/party переиспользует ОДИН и тот
     же компонент страницы, и без ключа карточки концертов остались бы в
     состоянии рядом с вечеринками. -->
{#key `${data.city.slug}/${data.section?.slug ?? ''}`}
  <Listing {...data} />
{/key}
