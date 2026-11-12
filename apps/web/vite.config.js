import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { VitePWA } from 'vite-plugin-pwa'

// `vite build --mode demo`: static demo for GitHub Pages (mock API, relative paths, no service worker)
export default defineConfig(({ mode }) => ({
  ...(mode === 'demo' && { base: './' }),
  plugins: [
    svelte(),
    mode !== 'demo' && VitePWA({
      registerType: 'autoUpdate',
      includeAssets: ['apple-touch-icon.png', 'favicon.png'],
      manifest: {
        name: 'Kantong',
        short_name: 'Kantong',
        start_url: '/',
        display: 'standalone',
        background_color: '#111214',
        theme_color: '#111214',
        icons: [
          { src: 'icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: 'icon-512.png', sizes: '512x512', type: 'image/png' },
          { src: 'icon-maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      workbox: {
        // App shell only; data always comes from the network.
        globPatterns: ['**/*.{js,css,html,png}'],
        navigateFallbackDenylist: [/^\/api\//, /^\/healthz/],
        runtimeCaching: [{ urlPattern: ({ url }) => url.pathname.startsWith('/api/'), handler: 'NetworkOnly' }],
      },
    }),
  ],
  // Go embeds this dir (apps/api/main.go: go:embed all:web/dist)
  build: mode === 'demo' ? { outDir: 'demo/dist', emptyOutDir: true } : { outDir: '../api/web/dist', emptyOutDir: true },
  server: { proxy: { '/api': 'http://127.0.0.1:8080' } },
}))
