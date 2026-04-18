import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e',
  use: { baseURL: process.env.BASE_URL ?? 'http://localhost:4173' },
  projects: [
    { name: 'chromium', use: devices['Desktop Chrome'] },
    { name: 'mobile-safari', use: devices['iPhone 15'] }
  ],
  webServer: process.env.BASE_URL ? undefined : {
    command: 'npm run preview',
    port: 4173,
    reuseExistingServer: false
  }
});
