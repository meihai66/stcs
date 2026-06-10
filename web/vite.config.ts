import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// 开发时把 /api、/outputs、/v1 代理到本地 Go 服务(默认 5311)。
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5312,
    proxy: {
      '/api': 'http://127.0.0.1:5311',
      '/outputs': 'http://127.0.0.1:5311',
      '/v1': 'http://127.0.0.1:5311',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
