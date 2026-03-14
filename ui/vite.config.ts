import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import type { UserConfig } from 'vite';

const config: UserConfig = {
  plugins: [tailwindcss(), sveltekit()],
  server: {
    proxy: {
      '/api': {
        target: process.env.UI_API_PROXY_URL || 'http://127.0.0.1:4173',
        changeOrigin: true,
        ws: true
      }
    }
  }
};

export default config;
