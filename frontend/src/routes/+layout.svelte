<script lang="ts">
  import '../app.css';
  import Scanlines from '$lib/components/Scanlines.svelte';
  let { children, data } = $props();

  /**
   * Счётчик Метрики. Собирается строкой и вставляется через {@html} по той же
   * причине, что и schema.org: содержимое <script> Svelte разбирает как код
   * компонента, а нам нужен текст для браузера. Закрывающий тег экранирован —
   * без этого он закрыл бы <script> самого компонента.
   *
   * Номер уже проверен на «только цифры» в +layout.server.ts.
   */
  const metrika = $derived(
    data?.metrikaId
      ? `(function(m,e,t,r,i,k,a){m[i]=m[i]||function(){(m[i].a=m[i].a||[]).push(arguments)};` +
        `m[i].l=1*new Date();for(var j=0;j<document.scripts.length;j++){if(document.scripts[j].src===r){return;}}` +
        `k=e.createElement(t),a=e.getElementsByTagName(t)[0],k.async=1,k.src=r,a.parentNode.insertBefore(k,a)})` +
        `(window,document,'script','https://mc.yandex.ru/metrika/tag.js?id=${data.metrikaId}','ym');` +
        `ym(${data.metrikaId},'init',{ssr:true,webvisor:true,clickmap:true,ecommerce:"dataLayer",` +
        `referrer:document.referrer,url:location.href,accurateTrackBounce:true,trackLinks:true});`
      : ''
  );
</script>

<svelte:head>
  <!-- Метки владения сайтом. Значение приезжает из окружения (см.
       +layout.server.ts); пустое — тега нет вовсе. -->
  {#if data?.verification?.yandex}
    <meta name="yandex-verification" content={data.verification.yandex} />
  {/if}
  {#if data?.verification?.google}
    <meta name="google-site-verification" content={data.verification.google} />
  {/if}
  {#if metrika}
    {@html `<script type="text/javascript">${metrika}<\/script>`}
  {/if}
</svelte:head>

<Scanlines />
{@render children?.()}

{#if metrika}
  <!-- Пиксель для посетителей без JS. Он ОБЯЗАН быть в теле, а не в head:
       <noscript> в head допускает только ссылки и мета, картинка оттуда не
       нарисуется, и такие посетители не сосчитаются вовсе. -->
  <noscript
    ><div>
      <img
        src={`https://mc.yandex.ru/watch/${data.metrikaId}`}
        style="position:absolute; left:-9999px;"
        alt=""
      />
    </div></noscript
  >
{/if}
