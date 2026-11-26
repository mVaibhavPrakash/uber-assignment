import { defineConfig } from 'vite';

export default defineConfig(({ mode }) => {
  const sample = mode === 'rain-demo';
  const server = {
    host: '127.0.0.1',
    port: sample ? 3001 : 3000,
    strictPort: true,
    proxy: { '/api': sample ? 'http://127.0.0.1:8081' : 'http://127.0.0.1:8080' },
  };
  return { server, preview: server };
});
