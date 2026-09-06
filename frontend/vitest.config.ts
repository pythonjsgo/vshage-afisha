import { defineConfig } from 'vitest/config';
import { sveltekit } from '@sveltejs/kit/vite';

export default defineConfig({
  plugins: [sveltekit()],
  test: {
    environment: 'jsdom',
    // Две строки, а не одна: тесты живут и в tests/unit/, и рядом с модулем
    // (src/lib/*.test.ts). Пока второй строки не было, файл рядом с модулем
    // не выполнялся ВООБЩЕ — он лежал в дереве, читался как защита и не
    // проверял ничего. Невыполненный тест хуже отсутствующего.
    include: ['tests/unit/**/*.{test,spec}.{js,ts}', 'src/**/*.{test,spec}.{js,ts}']
  }
});
