import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 设 MOCK_API=1 时把 /api 代理到 mock 服务（cmd/mockserver），用于前端独立开发
const apiTarget = process.env.MOCK_API === '1'
  ? 'http://localhost:8081'
  : 'http://localhost:8080'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': apiTarget,
    },
  },
})
