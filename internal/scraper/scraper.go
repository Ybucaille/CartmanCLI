// internal/scraper/scraper.go
package scraper

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://south-park-tv.fr"

var seasonSlugs = map[int]string{
	1:  "saison-1-i028g0lijp",
	2:  "saison-2",
	3:  "saison-3",
	4:  "saison-4",
	5:  "saison-5-4fy5o6k10s",
	6:  "saison-6-garf23590v",
	7:  "saison-7-garf23590v",
	8:  "saison-8-4fy5o6k10s",
	9:  "saison-9-garf23590v",
	10: "saison-10-garf23590v",
	11: "saison-11-4fy5o6k10s",
	12: "saison-12-garf23590v",
	13: "saison-13-4fy5o6k10s",
	14: "saison-14-0td16100mc",
	15: "saison-15-4fy5o6k10s",
	16: "saison-16-garf23590v",
	17: "saison-17-garf23590v",
	18: "saison-18-garf23590v",
	19: "saison-19-4aw7e7neet",
	20: "saison-20-garf23590v",
	21: "saison-21-garf23590v",
	22: "saison-22-garf23590v",
	23: "saison-23-garf23590v",
	24: "saison-24-garf23590v",
	25: "saison-25-garf23590v",
	26: "saison-26-garf23590v",
	27: "saison-27",
	28: "saison-28",
}

type Episode struct {
	Season int
	Number int
	Title  string
	URL    string
}

var (
	anchorRe = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']+)["'][^>]*>(.*?)</a>`)

	tagStripRe = regexp.MustCompile(`(?is)<[^>]+>`)
	spaceRe    = regexp.MustCompile(`\s+`)

	episodeTextRe = regexp.MustCompile(`(?i)\bs\s*0*([0-9]{1,2})\s*(?:e|episode|épisode)\s*0*([0-9]{1,3})\b`)
	episodeURLRe  = regexp.MustCompile(`(?i)\bs0*([0-9]{1,2})[-_\s]*episode[-_\s]*0*([0-9]{1,3})\b`)

	mediaTagAttrRe = regexp.MustCompile(`(?is)<(?:iframe|video|source|embed)[^>]+(?:src|data-src|data-lazy-src|data-rocket-src)=["']([^"']+)["']`)
	anyURLAttrRe   = regexp.MustCompile(`(?is)(?:href|src|data-src|data-lazy-src|data-rocket-src)=["']([^"']+)["']`)
)

func BuildSeasonURL(season int) (string, error) {
	slug, ok := seasonSlugs[season]
	if !ok {
		return "", fmt.Errorf("saison %d inconnue", season)
	}

	return fmt.Sprintf("%s/%s/", baseURL, slug), nil
}

func BuildEpisodeURL(season, episode int) (string, error) {
	slug, ok := seasonSlugs[season]
	if !ok {
		return "", fmt.Errorf("saison %d inconnue", season)
	}

	return fmt.Sprintf(
		"%s/%s/s%d-episode-%d/",
		baseURL,
		slug,
		season,
		episode,
	), nil
}

// ResolveEpisodeURL cherche d'abord l'URL réelle depuis la page de saison.
// Si le scraping ne trouve rien, on retombe sur BuildEpisodeURL.
func ResolveEpisodeURL(season, episode int) (string, error) {
	episodes, err := GetSeasonEpisodes(season)
	if err == nil {
		for _, ep := range episodes {
			if ep.Number == episode {
				return ep.URL, nil
			}
		}
	}

	fallback, fallbackErr := BuildEpisodeURL(season, episode)
	if fallbackErr != nil {
		return "", fallbackErr
	}

	return fallback, err
}

// GetSeasonEpisodes scrape la saison et ses pages paginées.
// Ça évite de supposer que toutes les saisons ont exactement la même structure.
func GetSeasonEpisodes(season int) ([]Episode, error) {
	seasonURL, err := BuildSeasonURL(season)
	if err != nil {
		return nil, err
	}

	seen := map[int]Episode{}

	for page := 1; page <= 8; page++ {
		pageURL := seasonURL
		if page > 1 {
			pageURL = fmt.Sprintf("%spage/%d/", seasonURL, page)
		}

		body, err := fetch(pageURL)
		if err != nil {
			if page == 1 {
				return nil, err
			}

			break
		}

		episodes := extractEpisodes(body, pageURL, season)
		if len(episodes) == 0 && page > 1 {
			break
		}

		for _, ep := range episodes {
			current, exists := seen[ep.Number]
			if !exists || len(ep.Title) > len(current.Title) {
				seen[ep.Number] = ep
			}
		}
	}

	if len(seen) == 0 {
		return nil, fmt.Errorf("aucun épisode trouvé pour la saison %d", season)
	}

	episodes := make([]Episode, 0, len(seen))
	for _, ep := range seen {
		episodes = append(episodes, ep)
	}

	sort.Slice(episodes, func(i, j int) bool {
		return episodes[i].Number < episodes[j].Number
	})

	return episodes, nil
}

func GetEmbedURL(pageURL string) (string, error) {
	body, err := fetch(pageURL)
	if err != nil {
		return "", err
	}

	candidates := collectVideoCandidates(body, pageURL)
	if len(candidates) == 0 {
		return "", fmt.Errorf("aucun embed vidéo trouvé")
	}

	bestURL := ""
	bestScore := 0

	for _, candidate := range candidates {
		score := scoreVideoCandidate(candidate)

		if score > bestScore {
			bestURL = candidate
			bestScore = score
		}
	}

	if bestURL == "" || bestScore <= 0 {
		return "", fmt.Errorf("aucun embed vidéo fiable trouvé")
	}

	return bestURL, nil
}

func extractEpisodes(body []byte, pageURL string, season int) []Episode {
	matches := anchorRe.FindAllSubmatch(body, -1)

	seen := map[int]Episode{}

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		rawHref := cleanTextBytes(match[1])
		rawLabel := cleanHTML(string(match[2]))

		resolvedURL := resolveURL(pageURL, rawHref)
		if resolvedURL == "" {
			continue
		}

		foundSeason, foundEpisode, ok := parseEpisodeIdentity(rawLabel)
		if !ok {
			foundSeason, foundEpisode, ok = parseEpisodeIdentity(resolvedURL)
		}

		if !ok {
			continue
		}

		if foundSeason != season {
			continue
		}

		if foundEpisode <= 0 {
			continue
		}

		title := rawLabel
		if title == "" {
			title = fmt.Sprintf("S%02d Episode %d", season, foundEpisode)
		}

		current, exists := seen[foundEpisode]
		if !exists || len(title) > len(current.Title) {
			seen[foundEpisode] = Episode{
				Season: season,
				Number: foundEpisode,
				Title:  title,
				URL:    resolvedURL,
			}
		}
	}

	episodes := make([]Episode, 0, len(seen))
	for _, ep := range seen {
		episodes = append(episodes, ep)
	}

	return episodes
}

func parseEpisodeIdentity(input string) (season, episode int, ok bool) {
	input = html.UnescapeString(input)
	input = strings.ToLower(input)
	input = strings.ReplaceAll(input, "%20", " ")
	input = strings.ReplaceAll(input, "_", "-")

	for _, re := range []*regexp.Regexp{episodeTextRe, episodeURLRe} {
		match := re.FindStringSubmatch(input)
		if len(match) < 3 {
			continue
		}

		s, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}

		e, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}

		return s, e, true
	}

	return 0, 0, false
}

func collectVideoCandidates(body []byte, pageURL string) []string {
	seen := map[string]bool{}
	candidates := []string{}

	add := func(raw string) {
		raw = strings.TrimSpace(html.UnescapeString(raw))
		if raw == "" {
			return
		}

		resolved := resolveURL(pageURL, raw)
		if resolved == "" {
			return
		}

		if seen[resolved] {
			return
		}

		seen[resolved] = true
		candidates = append(candidates, resolved)
	}

	for _, match := range mediaTagAttrRe.FindAllSubmatch(body, -1) {
		if len(match) > 1 {
			add(string(match[1]))
		}
	}

	for _, match := range anyURLAttrRe.FindAllSubmatch(body, -1) {
		if len(match) > 1 {
			raw := string(match[1])
			if looksLikeVideoURL(raw) {
				add(raw)
			}
		}
	}

	return candidates
}

func scoreVideoCandidate(raw string) int {
	u, err := url.Parse(raw)
	if err != nil {
		return -100
	}

	host := strings.ToLower(u.Hostname())
	path := strings.ToLower(u.EscapedPath())
	full := strings.ToLower(raw)

	if host == "" {
		return -100
	}

	if strings.HasPrefix(full, "javascript:") ||
		strings.HasPrefix(full, "data:") ||
		strings.HasPrefix(full, "mailto:") ||
		strings.Contains(full, "about:blank") {
		return -100
	}

	badHosts := []string{
		"google.",
		"gstatic.",
		"doubleclick.",
		"facebook.",
		"twitter.",
		"x.com",
		"wp.com",
		"gravatar.",
	}

	for _, bad := range badHosts {
		if strings.Contains(host, bad) {
			return -100
		}
	}

	score := 0

	knownVideoHosts := []string{
		"sibnet",
		"vidmoly",
		"voe.",
		"filemoon",
		"streamtape",
		"uqload",
		"dood",
		"ok.ru",
		"vk.com",
		"sendvid",
		"vidoza",
		"luluvdo",
		"youtube",
		"youtu.be",
		"dailymotion",
	}

	for _, provider := range knownVideoHosts {
		if strings.Contains(host, provider) {
			score += 100
			break
		}
	}

	videoHints := []string{
		"embed",
		"player",
		"video",
		"stream",
		"watch",
		"shell.php",
		"playlist",
		"m3u8",
	}

	for _, hint := range videoHints {
		if strings.Contains(path, hint) || strings.Contains(full, hint) {
			score += 25
		}
	}

	videoExts := []string{
		".mp4",
		".m3u8",
		".webm",
		".mkv",
		".avi",
		".mov",
	}

	for _, ext := range videoExts {
		if strings.Contains(path, ext) {
			score += 80
			break
		}
	}

	if strings.Contains(host, "south-park-tv.fr") {
		score -= 30
	}

	return score
}

func looksLikeVideoURL(raw string) bool {
	raw = strings.ToLower(raw)

	hints := []string{
		"sibnet",
		"vidmoly",
		"voe.",
		"filemoon",
		"streamtape",
		"uqload",
		"dood",
		"ok.ru",
		"vk.com",
		"sendvid",
		"vidoza",
		"luluvdo",
		"embed",
		"player",
		"video",
		"stream",
		"shell.php",
		".mp4",
		".m3u8",
		".webm",
	}

	for _, hint := range hints {
		if strings.Contains(raw, hint) {
			return true
		}
	}

	return false
}

func resolveURL(pageURL, rawHref string) string {
	rawHref = strings.TrimSpace(html.UnescapeString(rawHref))
	if rawHref == "" {
		return ""
	}

	if strings.HasPrefix(rawHref, "//") {
		return "https:" + rawHref
	}

	parsedHref, err := url.Parse(rawHref)
	if err != nil {
		return ""
	}

	if parsedHref.IsAbs() {
		return parsedHref.String()
	}

	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}

	return base.ResolveReference(parsedHref).String()
}

func cleanTextBytes(input []byte) string {
	return strings.TrimSpace(html.UnescapeString(string(input)))
}

func cleanHTML(input string) string {
	input = html.UnescapeString(input)
	input = tagStripRe.ReplaceAllString(input, " ")
	input = spaceRe.ReplaceAllString(input, " ")

	return strings.TrimSpace(input)
}

func fetch(rawURL string) ([]byte, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "fr-FR,fr;q=0.9,en;q=0.8")

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status: %s", res.Status)
	}

	return io.ReadAll(io.LimitReader(res.Body, 10*1024*1024))
}