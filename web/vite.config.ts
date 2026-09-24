import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';
import { mockApi } from './mock/plugin.ts';

export default defineConfig(({ mode }) => {
	const useMock = process.env.VITE_MOCK === '1' || mode === 'mock';
	return {
		plugins: [tailwindcss(), ...(useMock ? [mockApi()] : []), sveltekit()],
		server: {
			host: true,
			port: 5173,
			proxy: useMock
				? undefined
				: {
						'/api': { target: process.env.API_URL ?? 'http://localhost:8080', changeOrigin: false },
						'/healthz': { target: process.env.API_URL ?? 'http://localhost:8080', changeOrigin: false }
					}
		}
	};
});
