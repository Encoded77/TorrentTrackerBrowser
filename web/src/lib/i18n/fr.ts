// French dictionary: the reference shape every other locale must match (see en.ts).
// Code and identifiers stay in English; only the values are translated.

const NB = ' '; // French typography puts a non-breaking space before units and some punctuation.

export const fr = {
	localeTag: 'fr-FR',
	localeName: { fr: 'Français', en: 'English' } as Record<string, string>,
	chooseLanguage: 'Langue',

	appName: 'TorrentTrackerBrowser',
	appTagline: 'Recherche multi-trackers',
	pageTitle: 'TorrentTrackerBrowser, recherche multi-trackers',

	// Header
	toggleTheme: 'Changer le thème',
	openQueue: 'Ouvrir la file',
	queue: 'File',

	// Search bar
	searchPlaceholder: 'Rechercher un torrent…',
	searchLabel: 'Recherche',
	search: 'Rechercher',
	cancel: 'Annuler',
	cancelSearch: 'Annuler la recherche',
	pasteOrDrop: 'Coller un magnet / déposer un .torrent',
	pasteShort: 'Magnet / .torrent',
	allIndexers: 'Tous les indexeurs',
	indexersSelected: (n: number) => (n === 1 ? '1 indexeur' : `${n} indexeurs`),
	indexers: 'Indexeurs',
	indexerSearch: 'Filtrer les indexeurs…',
	noIndexer: 'Aucun indexeur',
	selectAll: 'Tout',
	selectNone: 'Aucun',
	private: 'privé',
	public: 'public',
	categories: 'Catégories',

	// Categories
	category: {
		movies: 'Films',
		tv: 'Séries',
		anime: 'Anime',
		music: 'Musique',
		books: 'Livres',
		games: 'Jeux',
		software: 'Logiciels',
		other: 'Autre'
	} as Record<string, string>,

	// Languages
	language: {
		fr: 'FR',
		multi: 'MULTI',
		vostfr: 'VOSTFR',
		en: 'EN'
	} as Record<string, string>,
	languageLong: {
		fr: 'Français',
		multi: 'Multi-langues',
		vostfr: 'VO sous-titrée',
		en: 'Anglais'
	} as Record<string, string>,

	// Status strip
	pending: 'en attente',
	failed: 'échec',
	resultsCount: (n: number) => (n === 1 ? '1 résultat' : `${n} résultats`),
	resultsShown: (shown: number, total: number) =>
		shown === total ? fr.resultsCount(total) : `${shown} affichés sur ${total}`,
	searching: 'Recherche en cours',
	searchDone: (ms: number) => `Terminé en ${(ms / 1000).toFixed(1).replace('.', ',')}${NB}s`,
	indexerFailed: (name: string) => `${name}${NB}: échec`,
	elapsedMs: (ms: number) => `${ms}${NB}ms`,

	// Results table
	colTitle: 'Titre',
	colIndexer: 'Indexeur',
	colSize: 'Taille',
	colSeeders: 'Sources',
	colLeechers: 'Pairs',
	colAge: 'Âge',
	colGrabs: 'Prises',
	colActions: 'Actions',
	selectRow: 'Sélectionner cette ligne',
	selectAllRows: 'Sélectionner toutes les lignes',
	sortBy: (col: string) => `Trier par ${col.toLowerCase()}`,
	freeleech: 'Freeleech',
	cached: 'En cache',
	cachedOn: (engine: string) => `En cache sur ${engine}`,
	notCached: 'Pas en cache',
	seenOn: (n: number) => `Vu sur ${n} indexeurs`,
	download: 'Télécharger',
	downloadSelection: (n: number) => `Télécharger la sélection (${n})`,
	clearSelection: 'Vider la sélection',
	copyMagnet: 'Copier le magnet',
	magnetCopied: 'Magnet copié',
	downloadTorrent: 'Télécharger le .torrent',
	previewFiles: 'Aperçu des fichiers',
	hidePreview: "Masquer l'aperçu",
	openInfoPage: 'Page du torrent',
	moreActions: "Plus d'actions",
	noPreview: 'Aperçu indisponible pour ce résultat.',
	filesLoading: 'Chargement des fichiers…',
	filesCount: (n: number) => (n === 1 ? '1 fichier' : `${n} fichiers`),
	filesSelected: (n: number, total: number) => `${n} sur ${total} fichiers sélectionnés`,
	selectFile: 'Sélectionner ce fichier',
	publishedOn: 'Publié le',
	grabs: 'prises',
	ago: (h: string) => `il y a ${h}`,

	// Filters
	filters: 'Filtres',
	filtersActive: (n: number) => (n === 1 ? '1 filtre actif' : `${n} filtres actifs`),
	minSeeders: 'Sources minimum',
	sizeMin: 'Taille min (Go)',
	sizeMax: 'Taille max (Go)',
	maxAge: 'Âge max (jours)',
	languages: 'Langues',
	cachedOnly: 'En cache seulement',
	resetFilters: 'Réinitialiser',
	anyValue: 'Indifférent',
	filterCategory: 'Catégorie',
	filterIndexer: 'Indexeur',

	// Empty / error states
	emptyTitle: 'Lancez une recherche',
	emptyBody: 'Tapez un titre, choisissez une catégorie ou des indexeurs, puis Entrée.',
	emptyHint: 'La touche / place le curseur dans la recherche, Échap annule.',
	noResultsTitle: 'Aucun résultat',
	noResultsBody: 'Aucun indexeur n’a renvoyé de résultat pour cette requête.',
	noResultsFiltered: (hidden: number) =>
		hidden === 1
			? '1 résultat est masqué par les filtres.'
			: `${hidden} résultats sont masqués par les filtres.`,
	searchErrorTitle: 'La recherche a échoué',
	searchStreamLost: 'La connexion au serveur a été perdue pendant la recherche.',
	loadingCapabilities: 'Connexion au serveur…',
	capabilitiesErrorTitle: 'Serveur injoignable',
	capabilitiesErrorBody: 'Impossible de charger la configuration. Vérifiez que le serveur tourne.',
	retry: 'Réessayer',

	// History
	history: 'Recherches récentes',
	savedSearches: 'Recherches enregistrées',
	saveSearch: 'Enregistrer cette recherche',
	unsaveSearch: 'Retirer des recherches enregistrées',
	clearHistory: "Effacer l'historique",
	removeFromHistory: "Retirer de l'historique",

	// Payload dialog
	payloadTitle: 'Ajouter un magnet ou un .torrent',
	payloadDescription: 'Collez un lien magnet, ou déposez un fichier .torrent.',
	magnetLabel: 'Lien magnet',
	magnetPlaceholder: 'magnet:?xt=urn:btih:…',
	dropZone: 'Déposez un fichier .torrent ici, ou cliquez pour en choisir un',
	dropZoneActive: 'Relâchez pour ajouter le fichier',
	fileChosen: (name: string) => `Fichier${NB}: ${name}`,
	invalidMagnet: 'Ce lien ne ressemble pas à un magnet (il doit commencer par magnet:?).',
	invalidTorrentFile: 'Seuls les fichiers .torrent sont acceptés.',
	continueToDownload: 'Continuer',
	payloadAdded: 'Torrent reconnu',

	// Download dialog
	downloadTitle: 'Télécharger',
	sendTitle: 'Envoyer vers un stockage',
	downloadItems: (n: number) => (n === 1 ? '1 élément' : `${n} éléments`),
	engine: 'Moteur',
	mode: 'Mode',
	modeCopy: 'Copier vers un stockage',
	modeAdopt: 'Conserver sur le moteur',
	modeLinks: 'Liens seulement (Mon PC)',
	modeDescription: {
		copy: 'Le moteur récupère le torrent, puis les fichiers sont copiés dans le stockage choisi.',
		adopt: 'Les fichiers restent là où le moteur les a écrits ; le stockage les adopte sans seconde copie.',
		links: 'Rien n’est copié : la file affiche des liens à ouvrir depuis votre PC.'
	} as Record<string, string>,
	storage: 'Stockage',
	free: (h: string) => `${h} libres`,
	subdir: 'Sous-dossier (optionnel)',
	subdirPlaceholder: 'ex. Films/2026',
	subdirHint: 'Caractères interdits retirés, pas de remontée de dossier.',
	files: 'Fichiers',
	allFiles: 'Tous les fichiers',
	launch: 'Lancer',
	launching: 'Lancement…',
	jobCreated: (name: string) => `Ajouté à la file${NB}: ${name}`,
	jobFailed: (name: string) => `Échec pour ${name}`,
	noEngine: 'Aucun moteur configuré.',
	noStorage: 'Aucun stockage configuré.',
	cachedCount: (n: number, total: number) => `${n} sur ${total} en cache`,

	// Queue
	queueTitle: 'File de téléchargement',
	queueDescription: 'Vos téléchargements, puis les éléments présents sur les moteurs.',
	queueEmpty: 'La file est vide',
	queueEmptyBody: 'Lancez un téléchargement depuis les résultats, ou collez un magnet.',
	queueError: 'Impossible de lire la file',
	active: (n: number) => (n === 1 ? '1 actif' : `${n} actifs`),
	external: 'externe',
	externalHint: 'Présent sur le moteur, mais non créé par cette application.',
	jobState: {
		queued: 'En attente',
		adding: 'Ajout',
		waitingSelection: 'Sélection requise',
		fetching: 'Récupération',
		ready: 'Prêt',
		copying: 'Copie',
		done: 'Terminé',
		failed: 'Échec',
		cancelled: 'Annulé'
	} as Record<string, string>,
	fileState: {
		pending: 'en attente',
		copying: 'copie',
		done: 'terminé',
		failed: 'échec',
		skipped: 'ignoré'
	} as Record<string, string>,
	cancelJob: 'Annuler',
	retryJob: 'Réessayer',
	deleteJob: 'Supprimer',
	sendToStorage: 'Envoyer vers un stockage',
	openLink: 'Ouvrir',
	showFiles: 'Afficher les fichiers',
	hideFiles: 'Masquer les fichiers',
	confirmDeleteTitle: 'Supprimer de la file ?',
	confirmDeleteBody: (name: string) => `« ${name} » sera retiré de la file.`,
	deleteFilesToo: 'Supprimer aussi les fichiers sur le moteur',
	confirmDelete: 'Supprimer',
	jobCancelled: 'Téléchargement annulé',
	jobRetried: 'Téléchargement relancé',
	jobDeleted: 'Retiré de la file',
	eta: (h: string) => `reste ${h}`,
	speed: (h: string) => `${h}/s`,
	modeLabel: {
		copy: 'copie',
		adopt: 'adoption',
		links: 'liens'
	} as Record<string, string>,

	// Formatting (units and durations; the number itself goes through Intl.NumberFormat)
	sizeUnits: ['o', 'Ko', 'Mo', 'Go', 'To', 'Po'],
	sizeWithUnit: (n: string, unit: string) => `${n}${NB}${unit}`,
	percent: (n: number) => `${n}${NB}%`,
	durSeconds: (n: number) => `${n}${NB}s`,
	durMinutes: (n: number) => `${n}${NB}min`,
	durHours: (h: number, m: number) => (m ? `${h}${NB}h${NB}${String(m).padStart(2, '0')}` : `${h}${NB}h`),
	durDays: (n: number) => `${n}${NB}j`,
	ageMonths: (n: number) => `${n}${NB}mois`,
	ageYears: (n: number) => (n === 1 ? `1${NB}an` : `${n}${NB}ans`),

	// Generic
	skipToContent: 'Aller au contenu',
	close: 'Fermer',
	closeToast: 'Fermer la notification',
	confirm: 'Confirmer',
	unknownError: 'Erreur inconnue',
	networkError: 'Le serveur ne répond pas.',
	copyFailed: 'Copie impossible dans le presse-papiers'
};

export type Dictionary = typeof fr;
