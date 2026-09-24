// English dictionary. Typed against the French shape: a missing or extra key fails `npm run check`.

import type { Dictionary } from './fr';

export const en: Dictionary = {
	localeTag: 'en-US',
	localeName: { fr: 'Français', en: 'English' },
	chooseLanguage: 'Language',

	appName: 'TorrentTrackerBrowser',
	appTagline: 'Multi-tracker search',
	pageTitle: 'TorrentTrackerBrowser, multi-tracker search',

	// Header
	toggleTheme: 'Switch theme',
	openQueue: 'Open the queue',
	queue: 'Queue',

	// Search bar
	searchPlaceholder: 'Search for a torrent…',
	searchLabel: 'Search',
	search: 'Search',
	cancel: 'Cancel',
	cancelSearch: 'Cancel the search',
	pasteOrDrop: 'Paste a magnet / drop a .torrent',
	pasteShort: 'Magnet / .torrent',
	allIndexers: 'All indexers',
	indexersSelected: (n: number) => (n === 1 ? '1 indexer' : `${n} indexers`),
	indexers: 'Indexers',
	indexerSearch: 'Filter indexers…',
	noIndexer: 'No indexer',
	selectAll: 'All',
	selectNone: 'None',
	private: 'private',
	public: 'public',
	categories: 'Categories',

	// Categories
	category: {
		movies: 'Movies',
		tv: 'TV',
		anime: 'Anime',
		music: 'Music',
		books: 'Books',
		games: 'Games',
		software: 'Software',
		other: 'Other'
	},

	// Languages
	language: {
		fr: 'FR',
		multi: 'MULTI',
		vostfr: 'VOSTFR',
		en: 'EN'
	},
	languageLong: {
		fr: 'French',
		multi: 'Multi-language',
		vostfr: 'Original audio, French subtitles',
		en: 'English'
	},

	// Status strip
	pending: 'pending',
	failed: 'failed',
	resultsCount: (n: number) => (n === 1 ? '1 result' : `${n} results`),
	resultsShown: (shown: number, total: number) =>
		shown === total ? en.resultsCount(total) : `${shown} shown of ${total}`,
	searching: 'Searching',
	searchDone: (ms: number) => `Done in ${(ms / 1000).toFixed(1)} s`,
	indexerFailed: (name: string) => `${name}: failed`,
	elapsedMs: (ms: number) => `${ms} ms`,

	// Results table
	colTitle: 'Title',
	colIndexer: 'Indexer',
	colSize: 'Size',
	colSeeders: 'Seeders',
	colLeechers: 'Leechers',
	colAge: 'Age',
	colGrabs: 'Grabs',
	colActions: 'Actions',
	selectRow: 'Select this row',
	selectAllRows: 'Select all rows',
	sortBy: (col: string) => `Sort by ${col.toLowerCase()}`,
	freeleech: 'Freeleech',
	cached: 'Cached',
	cachedOn: (engine: string) => `Cached on ${engine}`,
	notCached: 'Not cached',
	seenOn: (n: number) => `Seen on ${n} indexers`,
	download: 'Download',
	downloadSelection: (n: number) => `Download the selection (${n})`,
	clearSelection: 'Clear the selection',
	copyMagnet: 'Copy the magnet',
	magnetCopied: 'Magnet copied',
	downloadTorrent: 'Download the .torrent',
	previewFiles: 'Preview files',
	hidePreview: 'Hide the preview',
	openInfoPage: 'Torrent page',
	moreActions: 'More actions',
	noPreview: 'No preview available for this result.',
	filesLoading: 'Loading files…',
	filesCount: (n: number) => (n === 1 ? '1 file' : `${n} files`),
	filesSelected: (n: number, total: number) => `${n} of ${total} files selected`,
	selectFile: 'Select this file',
	publishedOn: 'Published on',
	grabs: 'grabs',
	ago: (h: string) => `${h} ago`,

	// Filters
	filters: 'Filters',
	filtersActive: (n: number) => (n === 1 ? '1 active filter' : `${n} active filters`),
	minSeeders: 'Minimum seeders',
	sizeMin: 'Min size (GB)',
	sizeMax: 'Max size (GB)',
	maxAge: 'Max age (days)',
	languages: 'Languages',
	cachedOnly: 'Cached only',
	resetFilters: 'Reset',
	anyValue: 'Any',
	filterCategory: 'Category',
	filterIndexer: 'Indexer',

	// Empty / error states
	emptyTitle: 'Start a search',
	emptyBody: 'Type a title, pick a category or some indexers, then press Enter.',
	emptyHint: 'The / key focuses the search box, Escape cancels.',
	noResultsTitle: 'No results',
	noResultsBody: 'No indexer returned a result for this query.',
	noResultsFiltered: (hidden: number) =>
		hidden === 1 ? '1 result is hidden by the filters.' : `${hidden} results are hidden by the filters.`,
	searchErrorTitle: 'The search failed',
	searchStreamLost: 'The connection to the server was lost during the search.',
	loadingCapabilities: 'Connecting to the server…',
	capabilitiesErrorTitle: 'Server unreachable',
	capabilitiesErrorBody: 'The configuration could not be loaded. Check that the server is running.',
	retry: 'Retry',

	// History
	history: 'Recent searches',
	savedSearches: 'Saved searches',
	saveSearch: 'Save this search',
	unsaveSearch: 'Remove from saved searches',
	clearHistory: 'Clear history',
	removeFromHistory: 'Remove from history',

	// Payload dialog
	payloadTitle: 'Add a magnet or a .torrent',
	payloadDescription: 'Paste a magnet link, or drop a .torrent file.',
	magnetLabel: 'Magnet link',
	magnetPlaceholder: 'magnet:?xt=urn:btih:…',
	dropZone: 'Drop a .torrent file here, or click to pick one',
	dropZoneActive: 'Release to add the file',
	fileChosen: (name: string) => `File: ${name}`,
	invalidMagnet: 'This link does not look like a magnet (it must start with magnet:?).',
	invalidTorrentFile: 'Only .torrent files are accepted.',
	continueToDownload: 'Continue',
	payloadAdded: 'Torrent recognised',

	// Download dialog
	downloadTitle: 'Download',
	sendTitle: 'Send to a storage',
	downloadItems: (n: number) => (n === 1 ? '1 item' : `${n} items`),
	engine: 'Engine',
	mode: 'Mode',
	modeCopy: 'Copy to a storage',
	modeAdopt: 'Keep on the engine',
	modeLinks: 'Links only (My PC)',
	modeDescription: {
		copy: 'The engine fetches the torrent, then the files are copied into the chosen storage.',
		adopt: 'The files stay where the engine wrote them; the storage adopts them without a second copy.',
		links: 'Nothing is copied: the queue shows links to open from your PC.'
	},
	storage: 'Storage',
	free: (h: string) => `${h} free`,
	subdir: 'Subfolder (optional)',
	subdirPlaceholder: 'e.g. Movies/2026',
	subdirHint: 'Forbidden characters are removed, no parent folder access.',
	files: 'Files',
	allFiles: 'All files',
	launch: 'Start',
	launching: 'Starting…',
	jobCreated: (name: string) => `Added to the queue: ${name}`,
	jobFailed: (name: string) => `Failed for ${name}`,
	noEngine: 'No engine configured.',
	noStorage: 'No storage configured.',
	cachedCount: (n: number, total: number) => `${n} of ${total} cached`,

	// Queue
	queueTitle: 'Download queue',
	queueDescription: 'Your downloads, then the items present on the engines.',
	queueEmpty: 'The queue is empty',
	queueEmptyBody: 'Start a download from the results, or paste a magnet.',
	queueError: 'The queue could not be read',
	active: (n: number) => (n === 1 ? '1 active' : `${n} active`),
	external: 'external',
	externalHint: 'Present on the engine, but not created by this app.',
	jobState: {
		queued: 'Queued',
		adding: 'Adding',
		waitingSelection: 'Selection required',
		fetching: 'Fetching',
		ready: 'Ready',
		copying: 'Copying',
		done: 'Done',
		failed: 'Failed',
		cancelled: 'Cancelled'
	},
	fileState: {
		pending: 'pending',
		copying: 'copying',
		done: 'done',
		failed: 'failed',
		skipped: 'skipped'
	},
	cancelJob: 'Cancel',
	retryJob: 'Retry',
	deleteJob: 'Delete',
	sendToStorage: 'Send to a storage',
	openLink: 'Open',
	showFiles: 'Show files',
	hideFiles: 'Hide files',
	confirmDeleteTitle: 'Remove from the queue?',
	confirmDeleteBody: (name: string) => `“${name}” will be removed from the queue.`,
	deleteFilesToo: 'Also delete the files on the engine',
	confirmDelete: 'Delete',
	jobCancelled: 'Download cancelled',
	jobRetried: 'Download restarted',
	jobDeleted: 'Removed from the queue',
	eta: (h: string) => `${h} left`,
	speed: (h: string) => `${h}/s`,
	modeLabel: {
		copy: 'copy',
		adopt: 'adopt',
		links: 'links'
	},

	// Formatting
	sizeUnits: ['B', 'KB', 'MB', 'GB', 'TB', 'PB'],
	sizeWithUnit: (n: string, unit: string) => `${n} ${unit}`,
	percent: (n: number) => `${n}%`,
	durSeconds: (n: number) => `${n} s`,
	durMinutes: (n: number) => `${n} min`,
	durHours: (h: number, m: number) => (m ? `${h} h ${String(m).padStart(2, '0')}` : `${h} h`),
	durDays: (n: number) => `${n} d`,
	ageMonths: (n: number) => (n === 1 ? '1 month' : `${n} months`),
	ageYears: (n: number) => (n === 1 ? '1 year' : `${n} years`),

	// Generic
	skipToContent: 'Skip to content',
	close: 'Close',
	closeToast: 'Close the notification',
	confirm: 'Confirm',
	unknownError: 'Unknown error',
	networkError: 'The server is not responding.',
	copyFailed: 'Could not copy to the clipboard'
};
