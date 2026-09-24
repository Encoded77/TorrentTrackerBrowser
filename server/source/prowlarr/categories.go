package prowlarr

import "github.com/Encoded77/TorrentTrackerBrowser/server/core"

// Prowlarr (newznab) category ids per canonical category, used for queries.
var queryCategories = map[core.Canonical][]int{
	core.CatMovies:   {2000},
	core.CatTV:       {5000},
	core.CatAnime:    {5070},
	core.CatMusic:    {3000},
	core.CatBooks:    {7000},
	core.CatGames:    {1000, 4050},
	core.CatSoftware: {4000},
	core.CatOther:    {6000, 8000},
}

// toCanonical maps one Prowlarr category id onto the app vocabulary.
func toCanonical(id int) core.Canonical {
	switch {
	case id == 5070:
		return core.CatAnime
	case id == 4050:
		return core.CatGames
	case id >= 2000 && id < 3000:
		return core.CatMovies
	case id >= 5000 && id < 6000:
		return core.CatTV
	case id >= 3000 && id < 4000:
		return core.CatMusic
	case id >= 7000 && id < 8000:
		return core.CatBooks
	case id >= 1000 && id < 2000:
		return core.CatGames
	case id >= 4000 && id < 5000:
		return core.CatSoftware
	}
	return core.CatOther
}

// canonicalSet maps a list of Prowlarr categories to a deduplicated list of
// canonical categories in display order.
func canonicalSet(ids []int) []core.Canonical {
	seen := map[core.Canonical]bool{}
	for _, id := range ids {
		// Ids >= 100000 are indexer-specific custom categories that Prowlarr
		// echoes next to the standard one; they carry no extra information.
		if id >= 100000 || id <= 0 {
			continue
		}
		seen[toCanonical(id)] = true
	}
	if len(seen) == 0 {
		seen[core.CatOther] = true
	}
	out := []core.Canonical{}
	for _, c := range core.Categories {
		if seen[c] {
			out = append(out, c)
		}
	}
	return out
}
