// Captures the README screenshots against a running instance.
// Usage: [LOCALE=en|fr] node scripts/screenshots.mjs [baseUrl] [browserPath]
//   node scripts/screenshots.mjs http://localhost:5173
//   LOCALE=fr node scripts/screenshots.mjs https://torrents.example.lan
// Needs a Chromium-based browser on the machine (Chrome, Edge, Brave); pass its path
// as the second argument or set BROWSER_PATH. Writes docs/screenshots/*.png.
// LOCALE (default en) is written to localStorage before the page loads, so the UI and the
// labels the script clicks on are taken from the matching dictionary in src/lib/i18n/.
import puppeteer from 'puppeteer-core';
import { mkdirSync } from 'node:fs';
import { resolve } from 'node:path';
// Node 24 strips the type annotations from these plain TS modules at import time.
import { en } from '../src/lib/i18n/en.ts';
import { fr } from '../src/lib/i18n/fr.ts';

const dictionaries = { en, fr };
const localeCode = (process.env.LOCALE ?? 'en').toLowerCase();
const dict = dictionaries[localeCode];
if (!dict) throw new Error(`LOCALE must be one of ${Object.keys(dictionaries).join(', ')}, got "${localeCode}"`);

const base = process.argv[2] ?? 'http://localhost:5173';
const browserPath =
	process.argv[3] ??
	process.env.BROWSER_PATH ??
	'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe';
const out = resolve(import.meta.dirname, '../../docs/screenshots');
mkdirSync(out, { recursive: true });

const query = process.env.QUERY ?? 'dune';
// "Terminé en" / "Done in": the text before the number in the status strip once a search ends.
const doneMarker = dict.searchDone(0).replace(/\d.*$/, '').trim();

const browser = await puppeteer.launch({
	executablePath: browserPath,
	headless: true,
	args: ['--ignore-certificate-errors', '--hide-scrollbars']
});

async function open(scheme, viewport) {
	const page = await browser.newPage();
	await page.evaluateOnNewDocument((code) => {
		try {
			localStorage.setItem('locale', code);
		} catch {
			// storage unavailable: the app falls back to the browser language
		}
	}, localeCode);
	await page.setViewport(viewport);
	await page.emulateMediaFeatures([{ name: 'prefers-color-scheme', value: scheme }]);
	await page.goto(`${base}/?q=${encodeURIComponent(query)}`, { waitUntil: 'networkidle2' });
	await page.waitForFunction((marker) => document.body.innerText.includes(marker), { timeout: 60000 }, doneMarker);
	await page.waitForNetworkIdle({ idleTime: 800 });
	return page;
}

async function clickByLabel(page, label, nth = 0) {
	const handles = await page.$$(`button[aria-label="${label}"]`);
	if (!handles[nth]) throw new Error(`no button "${label}" #${nth}`);
	await handles[nth].click();
}

const desktop = { width: 1400, height: 900, deviceScaleFactor: 1 };
const pause = (ms) => new Promise((r) => setTimeout(r, ms));

// 1. Search results, dark and light.
let page = await open('dark', desktop);
await page.screenshot({ path: `${out}/search-dark.png` });
await page.close();

page = await open('light', desktop);
await page.screenshot({ path: `${out}/search-light.png` });

// 2. File preview: expand the first row whose preview can load.
const boxes = () => document.querySelectorAll('[role="checkbox"]').length;
const before = await page.evaluate(boxes);
for (let i = 0; i < 6; i++) {
	try {
		await clickByLabel(page, dict.previewFiles, i);
		await page.waitForFunction((n) => document.querySelectorAll('[role="checkbox"]').length > n, { timeout: 8000 }, before);
		break;
	} catch {
		await clickByLabel(page, dict.hidePreview, 0).catch(() => {});
	}
}
await pause(500);
await page.screenshot({ path: `${out}/preview-light.png` });
await page.close();

// 3. Download dialog (dark).
page = await open('dark', desktop);
await clickByLabel(page, dict.download, 0);
await page.waitForSelector('[role="dialog"]', { timeout: 10000 });
await pause(600);
await page.screenshot({ path: `${out}/download-dialog-dark.png` });
await page.keyboard.press('Escape');
await pause(400);

// 4. Queue sheet (dark).
await clickByLabel(page, dict.openQueue, 0);
await page.waitForSelector('[role="dialog"]', { timeout: 10000 });
await page.mouse.move(5, 5); // keep hover tooltips out of the shot
await pause(1500);
await page.screenshot({ path: `${out}/queue-dark.png` });
await page.close();

// 5. Phone width (dark).
page = await open('dark', { width: 390, height: 844, deviceScaleFactor: 2, isMobile: true, hasTouch: true });
await page.screenshot({ path: `${out}/mobile-dark.png` });
await page.close();

await browser.close();
console.log(`screenshots written to ${out} (locale ${localeCode}, ${base})`);
