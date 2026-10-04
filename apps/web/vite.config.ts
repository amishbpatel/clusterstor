import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit()],
  server: {
    proxy: {
      '/api': {
        target: process.env.CLUSTERSTOR_API_DEV_TARGET || 'http://localhost:8080',
        changeOrigin: true,
        ws: true
      }
    }
  }
});
