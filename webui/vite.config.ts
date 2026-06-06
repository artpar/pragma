import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: false,
    proxy: {
      '/api': {
        target: process.env.PRAGMA_WEB_API ?? 'http://127.0.0.1:4817',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: '../internal/web/static',
    emptyOutDir: true
  }
})
