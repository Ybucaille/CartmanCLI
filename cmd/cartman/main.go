package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"CartmanCLI/internal/history"
	"CartmanCLI/internal/scraper"
)

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
	if len(os.Args) == 2 && os.Args[1] == "resume" {
		runResumeMode()
		return
	}

	if len(os.Args) == 3 {
		runDirectMode()
		return
	}

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
		fmt.Println("Erreur TUI:", err)
		os.Exit(1)
	}
}

func runDirectMode() {
	season, err := strconv.Atoi(os.Args[1])
	if err != nil || season <= 0 {
		fmt.Println("Erreur: saison invalide")
		return
	}

	episode, err := strconv.Atoi(os.Args[2])
	if err != nil || episode <= 0 {
		fmt.Println("Erreur: épisode invalide")
		return
	}

	pageURL, err := scraper.ResolveEpisodeURL(season, episode)
	if err != nil {
		fmt.Println("URL réelle non trouvée, tentative fallback.")
	}

	if pageURL == "" {
		pageURL, err = scraper.BuildEpisodeURL(season, episode)
		if err != nil {
			fmt.Println("Erreur:", err)
			return
		}
	}

	fmt.Println("Page URL :", pageURL)

	embedURL, err := scraper.GetEmbedURL(pageURL)
	if err != nil {
		fmt.Println("Vidéo non détectée automatiquement.")
		fmt.Println("Ouvre la page directement :", pageURL)
		return
	}

	fmt.Println("Embed URL:", embedURL)

	_ = history.SaveLast(history.LastWatch{
		Season:   season,
		Episode:  episode,
		PageURL:  pageURL,
		EmbedURL: embedURL,
	})

	if err := openWithMPV(embedURL); err != nil {
		fmt.Println("Erreur mpv:", err)
	}
}

func runResumeMode() {
	last, err := history.LoadLast()
	if err != nil {
		fmt.Println("Aucun épisode à reprendre pour l’instant.")
		fmt.Println("Lance d’abord un épisode avec : go run ./cmd/cartman 1 1")
		return
	}

	if last.Season <= 0 || last.Episode <= 0 {
		fmt.Println("Historique invalide, impossible de reprendre.")
		return
	}

	target := preferredTarget(last.PageURL, last.EmbedURL)
	if strings.TrimSpace(target) == "" {
		fmt.Println("Historique trouvé, mais aucune URL exploitable.")
		return
	}

	fmt.Printf("Reprise Saison %d · Episode %d\n", last.Season, last.Episode)
	fmt.Println("URL:", target)

	if err := openWithMPV(target); err != nil {
		fmt.Println("Erreur mpv:", err)
	}
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
		content = "État inconnu\n"
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
	b.WriteString(subtitleStyle.Render("Choisis une saison"))
	b.WriteString("\n")

	start, end := visibleRange(m.cursor, len(seasons), 14)

	for i := start; i < end; i++ {
		season := seasons[i]

		line := fmt.Sprintf("  Saison %02d", season)
		if m.cursor == i {
			line = selectedStyle.Render("› Saison " + fmt.Sprintf("%02d", season))
		} else {
			line = normalStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	if start > 0 {
		b.WriteString(subtitleStyle.Render("  ↑ saisons précédentes"))
		b.WriteString("\n")
	}

	if end < len(seasons) {
		b.WriteString(subtitleStyle.Render("  ↓ saisons suivantes"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("↑/↓ ou j/k · Entrée valider · q quitter"))

	return boxStyle.Render(b.String())
}

func renderEpisodeView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("CartmanCLI"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("Saison %d sélectionnée", m.season)))
	b.WriteString("\n")

	start, end := visibleRange(m.cursor, 30, 14)

	for i := start; i < end; i++ {
		episode := i + 1

		line := fmt.Sprintf("  Episode %02d", episode)
		if m.cursor == i {
			line = selectedStyle.Render("› Episode " + fmt.Sprintf("%02d", episode))
		} else {
			line = normalStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	if start > 0 {
		b.WriteString(subtitleStyle.Render("  ↑ épisodes précédents"))
		b.WriteString("\n")
	}

	if end < 30 {
		b.WriteString(subtitleStyle.Render("  ↓ épisodes suivants"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("↑/↓ ou j/k · Entrée valider · b retour · q quitter"))

	return boxStyle.Render(b.String())
}

func renderResultView(m model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("CartmanCLI"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("Saison %d · Episode %d", m.season, m.episode)))
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString(normalStyle.Render("Recherche de la vidéo..."))
		b.WriteString("\n\n")
		b.WriteString(footerStyle.Render("Patience, Cartman inspecte les players sans favoritisme."))
		return boxStyle.Render(b.String())
	}

	if m.pageURL != "" {
		b.WriteString(normalStyle.Render("Page URL"))
		b.WriteString("\n")
		b.WriteString(urlStyle.Render(m.pageURL))
		b.WriteString("\n\n")
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render("Vidéo non détectée"))
		b.WriteString("\n")
		b.WriteString(normalStyle.Render(m.err.Error()))
		b.WriteString("\n\n")
		b.WriteString(footerStyle.Render("b retour · Entrée/q quitter"))
		return boxStyle.Render(b.String())
	}

	b.WriteString(normalStyle.Render("Embed URL"))
	b.WriteString("\n")
	b.WriteString(urlStyle.Render(m.embedURL))
	b.WriteString("\n\n")
	b.WriteString(footerStyle.Render("Entrée ouvrir avec mpv · b retour · q quitter"))

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
	b.WriteString(subtitleStyle.Render("Bienvenue dans ton terminal TV pas très catholique."))
	b.WriteString("\n\n")

	options := []string{}

	if m.hasLast {
		options = append(options, fmt.Sprintf("Reprendre Saison %d · Episode %d", m.lastWatch.Season, m.lastWatch.Episode))
	}

	options = append(options, "Choisir une saison")

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
	b.WriteString(footerStyle.Render("↑/↓ ou j/k · Entrée valider · q quitter"))

	return boxStyle.Render(b.String())
}