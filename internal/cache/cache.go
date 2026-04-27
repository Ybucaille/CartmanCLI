package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"CartmanCLI/internal/scraper"
)

const cacheFilename = "episodes.json"

func cacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, "cartmancli")
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}

	return path, nil
}

func CachePath() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, cacheFilename), nil
}

func LoadEpisodes() ([]scraper.Episode, error) {
	path, err := CachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var episodes []scraper.Episode
	if err := json.Unmarshal(data, &episodes); err != nil {
		return nil, err
	}

	sortEpisodes(episodes)

	return episodes, nil
}

func SaveEpisodes(episodes []scraper.Episode) error {
	path, err := CachePath()
	if err != nil {
		return err
	}

	sortEpisodes(episodes)

	data, err := json.MarshalIndent(episodes, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func BuildEpisodes(seasons []int, progress func(season int, episodeCount int, embedCount int, err error)) ([]scraper.Episode, error) {
	seen := map[string]scraper.Episode{}

	for _, season := range seasons {
		episodes, err := scraper.GetSeasonEpisodes(season)
		if err != nil {
			if progress != nil {
				progress(season, 0, 0, err)
			}
			continue
		}

		embedCount := 0

		for _, ep := range episodes {
			if strings.TrimSpace(ep.URL) != "" {
				embedURL, embedErr := scraper.GetEmbedURL(ep.URL)
				if embedErr == nil && strings.TrimSpace(embedURL) != "" {
					ep.EmbedURL = embedURL
					embedCount++
				}
			}

			key := episodeKey(ep.Season, ep.Number)
			seen[key] = ep
		}

		if progress != nil {
			progress(season, len(episodes), embedCount, nil)
		}
	}

	if len(seen) == 0 {
		return nil, fmt.Errorf("aucun épisode trouvé pendant la mise à jour du cache")
	}

	episodes := make([]scraper.Episode, 0, len(seen))
	for _, ep := range seen {
		episodes = append(episodes, ep)
	}

	sortEpisodes(episodes)

	return episodes, nil
}

func FindEpisode(season, episode int) (scraper.Episode, bool) {
	episodes, err := LoadEpisodes()
	if err != nil {
		return scraper.Episode{}, false
	}

	for _, ep := range episodes {
		if ep.Season == season && ep.Number == episode {
			return ep, true
		}
	}

	return scraper.Episode{}, false
}

func EpisodesBySeason(season int) ([]scraper.Episode, bool) {
	episodes, err := LoadEpisodes()
	if err != nil {
		return nil, false
	}

	filtered := []scraper.Episode{}

	for _, ep := range episodes {
		if ep.Season == season {
			filtered = append(filtered, ep)
		}
	}

	sortEpisodes(filtered)

	return filtered, len(filtered) > 0
}

func UpsertEpisode(updated scraper.Episode) error {
	episodes, err := LoadEpisodes()
	if err != nil {
		episodes = []scraper.Episode{}
	}

	found := false

	for i, ep := range episodes {
		if ep.Season == updated.Season && ep.Number == updated.Number {
			episodes[i] = updated
			found = true
			break
		}
	}

	if !found {
		episodes = append(episodes, updated)
	}

	return SaveEpisodes(episodes)
}

func ResetAll() error {
	dir, err := cacheDir()
	if err != nil {
		return err
	}

	return os.RemoveAll(dir)
}

func episodeKey(season, episode int) string {
	return fmt.Sprintf("%02d-%02d", season, episode)
}

func sortEpisodes(episodes []scraper.Episode) {
	sort.Slice(episodes, func(i, j int) bool {
		if episodes[i].Season == episodes[j].Season {
			return episodes[i].Number < episodes[j].Number
		}

		return episodes[i].Season < episodes[j].Season
	})
}
