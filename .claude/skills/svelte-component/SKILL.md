---
name: svelte-component
description: Add a new Svelte 5 component to vshage-afisha/frontend/src/lib/components. Enforces the runes pattern ($state, $derived, $effect, snippets), TypeScript-first, and the existing folder conventions. Use when adding a reusable piece of UI.
allowed-tools: Read, Write, Edit, Glob, Grep, Bash(npm:*)
argument-hint: <ComponentName>
---

# /svelte-component — add a Svelte 5 component

Argument: `$1` — `PascalCase` component name (e.g. `EventCard`, `RegistrationForm`).

## Pattern

Component lives at `frontend/src/lib/components/<Name>.svelte`. If it has multiple
internal pieces, use a folder: `frontend/src/lib/components/<Name>/<Name>.svelte`
+ `index.ts` re-export.

### Skeleton (Svelte 5 runes)

```svelte
<script lang="ts">
  import type { Snippet } from 'svelte';

  type Props = {
    title: string;
    /** Optional named slot via snippet */
    children?: Snippet;
    /** Variants — keep them as a union, not boolean flags */
    variant?: 'default' | 'compact' | 'featured';
  };

  let { title, children, variant = 'default' }: Props = $props();

  // Local state with runes — NEVER `let count = 0; $: doubled = count * 2;`
  let expanded = $state(false);
  const expandedLabel = $derived(expanded ? 'Свернуть' : 'Развернуть');

  $effect(() => {
    // Side effects here. Cleanup via returned function.
    return () => { /* cleanup */ };
  });

  function toggle() { expanded = !expanded; }
</script>

<article class="card variant-{variant}" data-expanded={expanded}>
  <h3>{title}</h3>
  {#if children}
    <div class="content">
      {@render children()}
    </div>
  {/if}
  <button onclick={toggle}>{expandedLabel}</button>
</article>

<style>
  .card { /* ... */ }
</style>
```

## Rules (enforce these)

- **Always TypeScript** — `<script lang="ts">`, typed `Props`
- **Runes only** — `$state`, `$derived`, `$effect`, `$props`, `$bindable`. NO
  `$:` reactive blocks (Svelte 4 syntax)
- **Snippets, not slots** — `Snippet` type + `{@render children()}`. NO `<slot />`
- **Event handlers**: `onclick={...}`, NOT `on:click={...}`
- **Russian for UI strings**, English for code identifiers
- **No CSS-in-JS libs** — scoped `<style>` block in the component

## Steps

1. Read 1-2 existing components for style alignment:
   `ls frontend/src/lib/components/` and pick the closest match
2. Write the component file (skeleton above)
3. If reusable across routes, also re-export from `frontend/src/lib/index.ts`
4. Use it in a route: `import <Name> from '$lib/components/<Name>.svelte';`
5. `cd frontend && npm run check` — must be green (svelte-check, no `any`)

## Don't

- Don't build forms with `bind:value` for non-trivial validation. Use a typed
  store or a small reducer for multi-field forms.
- Don't fetch in `onMount` — use SvelteKit `+page.ts` `load()` for SSR data
- Don't mix Svelte 4 lifecycle (`beforeUpdate`, `afterUpdate`) with runes
