package history

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type LastWatch struct {
	Season   int    `json:"season"`
	Episode  int    `json:"episode"`
	PageURL  string `json:"page_url"`
	EmbedURL string `json:"embed_url"`
}

func configDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, "cartmancli")
	err = os.MkdirAll(path, 0755)
	if err != nil {
		return "", err
	}

	return path, nil
}

func SaveLast(w LastWatch) error {
	dir, err := configDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "last.json"), data, 0644)
}

func LoadLast() (LastWatch, error) {
	dir, err := configDir()
	if err != nil {
		return LastWatch{}, err
	}

	data, err := os.ReadFile(filepath.Join(dir, "last.json"))
	if err != nil {
		return LastWatch{}, err
	}

	var w LastWatch
	err = json.Unmarshal(data, &w)

	return w, err
}

func WatchLaterDir() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, "mpv-watch-later")
	err = os.MkdirAll(path, 0755)
	if err != nil {
		return "", err
	}

	return path, nil
}
