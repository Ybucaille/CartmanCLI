package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"CartmanCLI/internal/cache"
	"CartmanCLI/internal/history"
	"CartmanCLI/internal/metadata"
	"CartmanCLI/internal/scraper"
)

const version = "v0.2.0-dev"

type step int

const (
	stepHome step = iota
	stepSeason
	stepEpisode
	stepResult
)

type model struct {
	step      step
	cursor    int
	season    int
	episode   int
	pageURL   string
	embedURL  string
	err       error
	width     int
	height    int
	loading   bool
	lastWatch history.LastWatch
	hasLast   bool
}

var seasons = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
	21, 22, 23, 24, 25, 26, 27, 28,
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("213")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			MarginBottom(1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 3).
			Width(42)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("82"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)

	urlStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("45"))
)

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		runTUIMode()
		return
	}

	switch args[0] {
	case "play":
		if len(args) != 3 {
			fmt.Println("Usage: cartman play <season> <episode>")
			return
		}

		season, episode, ok := parseSeasonEpisodeArgs(args[1], args[2])
		if !ok {
			return
		}

		runPlayMode(season, episode)

	case "resume":
		if len(args) != 1 {
			fmt.Println("Usage: cartman resume")
			return
		}

		runResumeMode()

	case "list":
		if len(args) != 2 {
			fmt.Println("Usage: cartman list <season>")
			return
		}

		season, err := strconv.Atoi(args[1])
		if err != nil || season <= 0 {
			fmt.Println("Error: invalid season")
			return
		}

		runListMode(season)

	case "search":
		if len(args) < 2 {
			fmt.Println(`Usage: cartman search "keyword"`)
			return
		}

		query := strings.Join(args[1:], " ")
		runSearchMode(query)

	case "update":
		if len(args) != 1 {
			fmt.Println("Usage: cartman update")
			return
		}

		runUpdateMode()

	case "reset":
		if len(args) > 2 {
			fmt.Println("Usage: cartman reset [--yes]")
			return
		}

		yes := len(args) == 2 && (args[1] == "--yes" || args[1] == "-y")
		runResetMode(yes)

	case "version", "--version", "-v":
		fmt.Println("CartmanCLI", version)

	case "help", "--help", "-h":
		printHelp()

	default:
		printUnknownCommand(args[0])
	}
}

func runTUIMode() {
	last, err := history.LoadLast()
	hasLast := err == nil && last.Season > 0 && last.Episode > 0

	p := tea.NewProgram(
		model{
			step:      stepHome,
			cursor:    0,
			lastWatch: last,
			hasLast:   hasLast,
		},
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Println("TUI error:", err)
		os.Exit(1)
	}
}

func parseSeasonEpisodeArgs(rawSeason, rawEpisode string) (int, int, bool) {
	season, err := strconv.Atoi(rawSeason)
	if err != nil || season <= 0 {
		fmt.Println("Error: invalid season")
		return 0, 0, false
	}

	episode, err := strconv.Atoi(rawEpisode)
	if err != nil || episode <= 0 {
		fmt.Println("Error: invalid episode")
		return 0, 0, false
	}

	return season, episode, true
}

func runPlayMode(season, episode int) {
	if cachedEpisode, ok := cache.FindEpisode(season, episode); ok {
		if strings.TrimSpace(cachedEpisode.EmbedURL) != "" {
			playCachedEpisode(cachedEpisode)
			return
		}

		if strings.TrimSpace(cachedEpisode.URL) != "" {
			playEpisode(season, episode, cachedEpisode.URL)
			return
		}
	}

	pageURL, err := scraper.ResolveEpisodeURL(season, episode)
	if err != nil {
		fmt.Println("Real episode URL not found in cache, trying live fallback.")
	}

	if pageURL == "" {
		pageURL, err = scraper.BuildEpisodeURL(season, episode)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
	}

	playEpisode(season, episode, pageURL)
}

func runResumeMode() {
	last, err := history.LoadLast()
	if err != nil {
		fmt.Println("No episode to resume yet.")
		fmt.Println("Start one first with: cartman play 1 1")
		return
	}

	if last.Season <= 0 || last.Episode <= 0 {
		fmt.Println("Invalid history data, unable to resume.")
		return
	}

	target := preferredTarget(last.PageURL, last.EmbedURL)
	if strings.TrimSpace(target) == "" {
		fmt.Println("History found, but no usable URL is available.")
		return
	}

	title := metadata.DisplayTitle(last.Season, last.Episode, "")

	fmt.Printf("Resuming S%02dE%02d · %s\n", last.Season, last.Episode, title)
	fmt.Println("URL:", target)

	if err := openWithMPV(target); err != nil {
		fmt.Println("mpv error:", err)
	}
}

func runResetMode(skipConfirm bool) {
	if !skipConfirm {
		fmt.Println("This command will delete all local CartmanCLI data:")
		fmt.Println("  - last watched episode")
		fmt.Println("  - mpv watch-later progress")
		fmt.Println("  - local episode cache")
		fmt.Println()
		fmt.Print(`Type "reset" to confirm, or press Enter to cancel > `)

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input != "reset" {
			fmt.Println("Reset cancelled.")
			return
		}
	}

	if err := history.ResetAll(); err != nil {
		fmt.Println("Failed to reset history:", err)
		return
	}

	if err := cache.ResetAll(); err != nil {
		fmt.Println("Failed to reset cache:", err)
		return
	}

	fmt.Println("CartmanCLI data deleted.")
}

func runUpdateMode() {
	p := tea.NewProgram(
		newUpdateModel(),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Println("Update TUI error:", err)
	}
}

type updateProgressMsg struct {
	Season       int
	EpisodeCount int
	EmbedCount   int
	Err          error
}

type updateDoneMsg struct {
	EpisodeCount int
	CachePath    string
	Err          error
}

type updateModel struct {
	width       int
	total       int
	current     int
	currentLine string
	done        bool
	err         error

	episodeCount int
	cachePath    string

	ch chan tea.Msg
}

func newUpdateModel() updateModel {
	return updateModel{
		total: len(seasons),
		ch:    make(chan tea.Msg, len(seasons)+2),
	}
}

func (m updateModel) Init() tea.Cmd {
	return tea.Batch(
		startUpdateCmd(m.ch),
		waitUpdateMsg(m.ch),
	)
}

func startUpdateCmd(ch chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			episodes, err := cache.BuildEpisodes(seasons, func(season int, episodeCount int, embedCount int, err error) {
				ch <- updateProgressMsg{
					Season:       season,
					EpisodeCount: episodeCount,
					EmbedCount:   embedCount,
					Err:          err,
				}
			})

			if err != nil {
				ch <- updateDoneMsg{
					Err: err,
				}
				return
			}

			if err := cache.SaveEpisodes(episodes); err != nil {
				ch <- updateDoneMsg{
					Err: err,
				}
				return
			}

			path, _ := cache.CachePath()

			ch <- updateDoneMsg{
				EpisodeCount: len(episodes),
				CachePath:    path,
				Err:          nil,
			}
		}()

		return nil
	}
}

func waitUpdateMsg(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func (m updateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit

		case "enter":
			if m.done {
				return m, tea.Quit
			}
		}

	case updateProgressMsg:
		m.current++

		if msg.Err != nil {
			m.currentLine = fmt.Sprintf("Current: Season %02d · error: %v", msg.Season, msg.Err)
		} else {
			m.currentLine = fmt.Sprintf(
				"Current: Season %02d · %d episodes · %d embeds",
				msg.Season,
				msg.EpisodeCount,
				msg.EmbedCount,
			)
		}

		return m, waitUpdateMsg(m.ch)

	case updateDoneMsg:
		m.done = true
		m.err = msg.Err
		m.episodeCount = msg.EpisodeCount
		m.cachePath = msg.CachePath

		if msg.Err != nil {
			m.currentLine = fmt.Sprintf("Update failed: %v", msg.Err)
		} else {
			m.current = m.total
			m.currentLine = "Update complete."
		}
	}

	return m, nil
}

func (m updateModel) View() string {
	var b strings.Builder

	percent := 0.0
	if m.total > 0 {
		percent = float64(m.current) / float64(m.total)
	}

	if percent > 1 {
		percent = 1
	}

	b.WriteString(titleStyle.Render("CartmanCLI Update"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Fetching episode pages and video embeds."))
	b.WriteString("\n\n")

	barWidth := 42
	if m.width > 0 && m.width < 70 {
		barWidth = 30
	}

	b.WriteString(renderProgressBar(percent, barWidth))
	b.WriteString("\n\n")

	b.WriteString(normalStyle.Render(fmt.Sprintf(
		"Progress: %d/%d seasons",
		m.current,
		m.total,
	)))
	b.WriteString("\n\n")

	if strings.TrimSpace(m.currentLine) != "" {
		if strings.Contains(strings.ToLower(m.currentLine), "error") ||
			strings.Contains(strings.ToLower(m.currentLine), "failed") {
			b.WriteString(errorStyle.Render(m.currentLine))
		} else {
			b.WriteString(normalStyle.Render(m.currentLine))
		}

		b.WriteString("\n\n")
	}

	if m.done {
		if m.err != nil {
			b.WriteString(errorStyle.Render("Update finished with errors."))
		} else {
			b.WriteString(selectedStyle.Render("Update complete."))
			b.WriteString("\n")
			b.WriteString(normalStyle.Render(fmt.Sprintf("Episodes saved: %d", m.episodeCount)))

			if strings.TrimSpace(m.cachePath) != "" {
				b.WriteString("\n")
				b.WriteString(normalStyle.Render("File: " + m.cachePath))
			}
		}

		b.WriteString("\n")
		b.WriteString(footerStyle.Render("Enter/q quit"))
	} else {
		b.WriteString(footerStyle.Render("q cancel"))
	}

	return lipgloss.Place(
		maxInt(m.width, 80),
		24,
		lipgloss.Center,
		lipgloss.Center,
		boxStyle.Width(74).Render(b.String()),
	)
}

func renderProgressBar(percent float64, width int) string {
	if width < 10 {
		width = 10
	}

	if percent < 0 {
		percent = 0
	}

	if percent > 1 {
		percent = 1
	}

	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}

	empty := width - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)

	return fmt.Sprintf("[%s] %3.0f%%", bar, percent*100)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func runListMode(season int) {
	episodes, ok := cache.EpisodesBySeason(season)

	if !ok {
		var err error
		episodes, err = scraper.GetSeasonEpisodes(season)
		if err != nil {
			fmt.Println("Error:", err)
			fmt.Println()
			fmt.Println("Tip: run `cartman update` to build the local cache.")
			return
		}
	}

	if len(episodes) == 0 {
		fmt.Printf("No episodes found for season %d.\n", season)
		return
	}

	fmt.Printf("Season %d\n\n", season)

	for _, ep := range episodes {
		title := metadata.DisplayTitle(ep.Season, ep.Number, ep.Title)

		fmt.Printf("S%02dE%02d · %s\n", ep.Season, ep.Number, title)
		fmt.Printf("  %s\n", ep.URL)
	}
}

func runSearchMode(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		fmt.Println(`Usage: cartman search "keyword"`)
		return
	}

	episodes, err := cache.LoadEpisodes()
	if err != nil || len(episodes) == 0 {
		fmt.Println("Local cache not found or empty.")
		fmt.Println("Run first: cartman update")
		return
	}

	normalizedQuery := normalizeSearch(query)
	matches := []scraper.Episode{}

	wantsSeason := 0
	wantsEpisode := 0

	if strings.HasPrefix(normalizedQuery, "s") && strings.Contains(normalizedQuery, "e") {
		raw := strings.TrimPrefix(normalizedQuery, "s")
		parts := strings.SplitN(raw, "e", 2)

		if len(parts) == 2 {
			s, sErr := atoiLoose(parts[0])
			e, eErr := atoiLoose(parts[1])

			if sErr == nil && eErr == nil && s > 0 && e > 0 {
				wantsSeason = s
				wantsEpisode = e
			}
		}
	}

	fmt.Printf("Searching for %q...\n\n", query)

	for _, ep := range episodes {
		searchable := metadata.SearchableText(ep.Season, ep.Number, ep.Title)
		searchable += " " + ep.URL
		searchable = normalizeSearch(searchable)

		if wantsSeason > 0 && wantsEpisode > 0 {
			if ep.Season == wantsSeason && ep.Number == wantsEpisode {
				matches = append(matches, ep)
			}

			continue
		}

		if strings.Contains(searchable, normalizedQuery) {
			matches = append(matches, ep)
		}
	}

	if len(matches) == 0 {
		fmt.Println("No episode found.")
		fmt.Println()
		fmt.Println(`Try for example: cartman search "s10e8" or cartman list 10`)
		return
	}

	for i, ep := range matches {
		title := metadata.DisplayTitle(ep.Season, ep.Number, ep.Title)

		fmt.Printf("%2d. S%02dE%02d · %s\n", i+1, ep.Season, ep.Number, title)
		fmt.Printf("    %s\n", ep.URL)
	}

	fmt.Println()
	fmt.Print("Enter the episode number to play, or press Enter to quit > ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		fmt.Println("Search closed.")
		return
	}

	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(matches) {
		fmt.Println("Invalid choice.")
		return
	}

	selected := matches[choice-1]
	playEpisode(selected.Season, selected.Number, selected.URL)
}

func normalizeSearch(input string) string {
	input = strings.ToLower(input)
	input = strings.TrimSpace(input)
	input = strings.ReplaceAll(input, "é", "e")
	input = strings.ReplaceAll(input, "è", "e")
	input = strings.ReplaceAll(input, "ê", "e")
	input = strings.ReplaceAll(input, "ë", "e")
	input = strings.ReplaceAll(input, "à", "a")
	input = strings.ReplaceAll(input, "â", "a")
	input = strings.ReplaceAll(input, "ä", "a")
	input = strings.ReplaceAll(input, "î", "i")
	input = strings.ReplaceAll(input, "ï", "i")
	input = strings.ReplaceAll(input, "ô", "o")
	input = strings.ReplaceAll(input, "ö", "o")
	input = strings.ReplaceAll(input, "ù", "u")
	input = strings.ReplaceAll(input, "û", "u")
	input = strings.ReplaceAll(input, "ü", "u")
	input = strings.ReplaceAll(input, "ç", "c")

	return input
}

func atoiLoose(input string) (int, error) {
	input = strings.TrimSpace(input)
	input = strings.TrimLeft(input, "0")

	if input == "" {
		input = "0"
	}

	return strconv.Atoi(input)
}

func playEpisode(season, episode int, pageURL string) {
	title := metadata.DisplayTitle(season, episode, "")

	fmt.Printf("Episode: S%02dE%02d · %s\n", season, episode, title)
	fmt.Println("Page URL:", pageURL)

	embedURL, err := scraper.GetEmbedURL(pageURL)
	if err != nil {
		fmt.Println("Video could not be detected automatically.")
		fmt.Println("Open the page directly:", pageURL)
		return
	}

	fmt.Println("Embed URL:", embedURL)

	ep := scraper.Episode{
		Season:   season,
		Number:   episode,
		Title:    title,
		URL:      pageURL,
		EmbedURL: embedURL,
	}

	_ = cache.UpsertEpisode(ep)

	_ = history.SaveLast(history.LastWatch{
		Season:   season,
		Episode:  episode,
		PageURL:  pageURL,
		EmbedURL: embedURL,
	})

	if err := openWithMPV(embedURL); err != nil {
		fmt.Println("mpv error:", err)
	}
}

func playCachedEpisode(ep scraper.Episode) {
	title := metadata.DisplayTitle(ep.Season, ep.Number, ep.Title)

	fmt.Printf("Episode: S%02dE%02d · %s\n", ep.Season, ep.Number, title)
	fmt.Println("Page URL:", ep.URL)
	fmt.Println("Embed URL:", ep.EmbedURL)

	_ = history.SaveLast(history.LastWatch{
		Season:   ep.Season,
		Episode:  ep.Number,
		PageURL:  ep.URL,
		EmbedURL: ep.EmbedURL,
	})

	err := openWithMPV(ep.EmbedURL)
	if err == nil {
		return
	}

	fmt.Println("mpv failed with cached embed:", err)
	fmt.Println("Refreshing embed from episode page...")

	if strings.TrimSpace(ep.URL) == "" {
		fmt.Println("Cannot refresh: page URL is empty.")
		return
	}

	freshEmbedURL, refreshErr := scraper.GetEmbedURL(ep.URL)
	if refreshErr != nil {
		fmt.Println("Refresh failed:", refreshErr)
		return
	}

	if strings.TrimSpace(freshEmbedURL) == "" {
		fmt.Println("Refresh failed: empty embed URL.")
		return
	}

	ep.EmbedURL = freshEmbedURL

	_ = cache.UpsertEpisode(ep)

	_ = history.SaveLast(history.LastWatch{
		Season:   ep.Season,
		Episode:  ep.Number,
		PageURL:  ep.URL,
		EmbedURL: ep.EmbedURL,
	})

	fmt.Println("New Embed URL:", ep.EmbedURL)

	if err := openWithMPV(ep.EmbedURL); err != nil {
		fmt.Println("mpv error after refresh:", err)
	}
}

func printHelp() {
	fmt.Println("CartmanCLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  cartman")
	fmt.Println("  cartman play <season> <episode>")
	fmt.Println("  cartman resume")
	fmt.Println("  cartman list <season>")
	fmt.Println(`  cartman search "keyword"`)
	fmt.Println("  cartman update")
	fmt.Println("  cartman reset")
	fmt.Println("  cartman version")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  cartman")
	fmt.Println("  cartman play 1 8")
	fmt.Println("  cartman resume")
	fmt.Println("  cartman list 7")
	fmt.Println(`  cartman search "warcraft"`)
	fmt.Println("  cartman update")
	fmt.Println("  cartman reset")
}

func printUnknownCommand(command string) {
	fmt.Println("Unknown command:", command)
	fmt.Println()
	printHelp()
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			max := m.maxCursor()
			if m.cursor < max {
				m.cursor++
			}

		case "enter":
			switch m.step {
			case stepHome:
				if m.hasLast && m.cursor == 0 {
					m.season = m.lastWatch.Season
					m.episode = m.lastWatch.Episode
					m.pageURL = m.lastWatch.PageURL
					m.embedURL = m.lastWatch.EmbedURL

					target := preferredTarget(m.lastWatch.PageURL, m.lastWatch.EmbedURL)

					return m, tea.Sequence(
						tea.ExitAltScreen,
						func() tea.Msg {
							err := openWithMPV(target)
							return mpvDoneMsg{err: err}
						},
					)
				}

				m.step = stepSeason
				m.cursor = 0

			case stepSeason:
				m.season = seasons[m.cursor]
				m.cursor = 0
				m.step = stepEpisode

			case stepEpisode:
				m.episode = m.cursor + 1
				m.step = stepResult
				m.loading = true
				m.pageURL = ""
				m.embedURL = ""
				m.err = nil
				return m, fetchEmbedCmd(m.season, m.episode)

			case stepResult:
				if m.err == nil && (m.pageURL != "" || m.embedURL != "") {
					return m, tea.Sequence(
						tea.ExitAltScreen,
						func() tea.Msg {
							target := preferredTarget(m.pageURL, m.embedURL)

							_ = history.SaveLast(history.LastWatch{
								Season:   m.season,
								Episode:  m.episode,
								PageURL:  m.pageURL,
								EmbedURL: m.embedURL,
							})

							err := openWithMPV(target)
							return mpvDoneMsg{err: err}
						},
					)
				}

				return m, tea.Quit
			}

		case "backspace", "left", "h", "b":
			switch m.step {
			case stepSeason:
				m.step = stepHome
				m.cursor = 0

			case stepEpisode:
				m.step = stepSeason
				m.cursor = indexOfSeason(m.season)

			case stepResult:
				m.step = stepEpisode
				m.cursor = m.episode - 1
				m.loading = false
				m.err = nil
			}
		}

	case embedResultMsg:
		m.pageURL = msg.pageURL
		m.embedURL = msg.embedURL
		m.err = msg.err
		m.loading = false

		if msg.err != nil {
			m.step = stepResult
			return m, nil
		}

		_ = history.SaveLast(history.LastWatch{
			Season:   m.season,
			Episode:  m.episode,
			PageURL:  msg.pageURL,
			EmbedURL: msg.embedURL,
		})

		return m, tea.Sequence(
			tea.ExitAltScreen,
			func() tea.Msg {
				target := preferredTarget(msg.pageURL, msg.embedURL)
				err := openWithMPV(target)
				return mpvDoneMsg{err: err}
			},
		)

	case mpvDoneMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("mpv: %w", msg.err)
		}
		return m, tea.Quit
	}

	return m, nil
}

func (m model) View() string {
	var content string

	switch m.step {
	case stepHome:
		content = renderHomeView(m)
	case stepSeason:
		content = renderSeasonView(m)
	case stepEpisode:
		content = renderEpisodeView(m)
	case stepResult:
		content = renderResultView(m)
	default:
		content = "Unknown state\n"
	}

	if m.width <= 0 || m.height <= 0 {
		return content
	}

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

func (m model) maxCursor() int {
	switch m.step {
	case stepHome:
		if m.hasLast {
			return 1
		}
		return 0
	case stepSeason:
		return len(seasons) - 1
	case stepEpisode:
		return 29
	default:
		return 0
	}
}

func renderSeasonView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("CartmanCLI"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Choose a season"))
	b.WriteString("\n")

	start, end := visibleRange(m.cursor, len(seasons), 14)

	for i := start; i < end; i++ {
		season := seasons[i]

		line := fmt.Sprintf("  Season %02d", season)
		if m.cursor == i {
			line = selectedStyle.Render("› Season " + fmt.Sprintf("%02d", season))
		} else {
			line = normalStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	if start > 0 {
		b.WriteString(subtitleStyle.Render("  ↑ previous seasons"))
		b.WriteString("\n")
	}

	if end < len(seasons) {
		b.WriteString(subtitleStyle.Render("  ↓ next seasons"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("↑/↓ or j/k · Enter select · q quit"))

	return boxStyle.Render(b.String())
}

func renderEpisodeView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("CartmanCLI"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("Season %d selected", m.season)))
	b.WriteString("\n")

	start, end := visibleRange(m.cursor, 30, 14)

	for i := start; i < end; i++ {
		episode := i + 1
		title := metadata.DisplayTitle(m.season, episode, "")

		line := fmt.Sprintf("  Episode %02d · %s", episode, title)
		if m.cursor == i {
			line = selectedStyle.Render("› Episode " + fmt.Sprintf("%02d", episode) + " · " + title)
		} else {
			line = normalStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	if start > 0 {
		b.WriteString(subtitleStyle.Render("  ↑ previous episodes"))
		b.WriteString("\n")
	}

	if end < 30 {
		b.WriteString(subtitleStyle.Render("  ↓ next episodes"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("↑/↓ or j/k · Enter select · b back · q quit"))

	return boxStyle.Width(72).Render(b.String())
}

func renderResultView(m model) string {
	var b strings.Builder

	title := metadata.DisplayTitle(m.season, m.episode, "")

	b.WriteString(titleStyle.Render("CartmanCLI"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("S%02dE%02d · %s", m.season, m.episode, title)))
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString(normalStyle.Render("Searching for video..."))
		b.WriteString("\n\n")
		b.WriteString(footerStyle.Render("Cartman is checking available players."))
		return boxStyle.Render(b.String())
	}

	if m.pageURL != "" {
		b.WriteString(normalStyle.Render("Page URL"))
		b.WriteString("\n")
		b.WriteString(urlStyle.Render(m.pageURL))
		b.WriteString("\n\n")
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render("Video not detected"))
		b.WriteString("\n")
		b.WriteString(normalStyle.Render(m.err.Error()))
		b.WriteString("\n\n")
		b.WriteString(footerStyle.Render("b back · Enter/q quit"))
		return boxStyle.Render(b.String())
	}

	b.WriteString(normalStyle.Render("Embed URL"))
	b.WriteString("\n")
	b.WriteString(urlStyle.Render(m.embedURL))
	b.WriteString("\n\n")
	b.WriteString(footerStyle.Render("Enter open with mpv · b back · q quit"))

	return boxStyle.Render(b.String())
}

func visibleRange(cursor, total, window int) (int, int) {
	if total <= window {
		return 0, total
	}

	half := window / 2
	start := cursor - half

	if start < 0 {
		start = 0
	}

	end := start + window

	if end > total {
		end = total
		start = end - window
	}

	return start, end
}

type embedResultMsg struct {
	pageURL  string
	embedURL string
	err      error
}

type mpvDoneMsg struct {
	err error
}

func fetchEmbedCmd(season, episode int) tea.Cmd {
	return func() tea.Msg {
		pageURL, err := scraper.ResolveEpisodeURL(season, episode)
		if pageURL == "" {
			return embedResultMsg{err: err}
		}

		embedURL, embedErr := scraper.GetEmbedURL(pageURL)
		if embedErr != nil {
			return embedResultMsg{
				pageURL:  pageURL,
				embedURL: "",
				err:      embedErr,
			}
		}

		return embedResultMsg{
			pageURL:  pageURL,
			embedURL: embedURL,
			err:      err,
		}
	}
}

func indexOfSeason(season int) int {
	for i, s := range seasons {
		if s == season {
			return i
		}
	}

	return 0
}

func preferredTarget(pageURL, embedURL string) string {
	if strings.TrimSpace(embedURL) != "" {
		return embedURL
	}

	return pageURL
}

func openWithMPV(url string) error {
	watchLaterDir, err := history.WatchLaterDir()
	if err != nil {
		return err
	}

	cmd := exec.Command(
		"mpv",
		"--force-window=yes",
		"--save-position-on-quit",
		"--watch-later-directory="+watchLaterDir,
		url,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func renderHomeView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("CartmanCLI"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Watch episodes from your terminal."))
	b.WriteString("\n\n")

	options := []string{}

	if m.hasLast {
		title := metadata.DisplayTitle(m.lastWatch.Season, m.lastWatch.Episode, "")
		options = append(options, fmt.Sprintf("Resume S%02dE%02d · %s", m.lastWatch.Season, m.lastWatch.Episode, title))
	}

	options = append(options, "Choose a season")

	for i, option := range options {
		line := "  " + option
		if m.cursor == i {
			line = selectedStyle.Render("› " + option)
		} else {
			line = normalStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("↑/↓ or j/k · Enter select · q quit"))

	return boxStyle.Width(72).Render(b.String())
}
