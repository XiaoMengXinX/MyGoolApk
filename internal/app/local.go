package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

type historyEntry struct {
	FeedID    string    `json:"feedID"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	AvatarURL string    `json:"avatarURL,omitempty"`
	ViewedAt  time.Time `json:"viewedAt"`
}

// localState is what the app keeps on the device: the browsing history, the
// recent searches, the appearance and the window.
type localState struct {
	History           []historyEntry `json:"history,omitempty"`
	HistoryEnabled    bool           `json:"historyEnabled"`
	ThemeMode         int            `json:"themeMode"` // 0 follows the system, 1 light, 2 dark
	FollowGroup       int            `json:"followGroup,omitempty"`
	RandomDeviceModel bool           `json:"randomDeviceModel,omitempty"`
	Searches          []string       `json:"searches,omitempty"`
	TradeCity         string         `json:"tradeCity,omitempty"` // the city of the second-hand posts nearby
	// Window is where a desktop's window was, to open there again.
	Window core.WindowFrame `json:"window,omitzero"`
}

func loadLocalState(dir string) localState {
	state := localState{HistoryEnabled: true}
	data, err := os.ReadFile(filepath.Join(dir, "local-state.json"))
	if err != nil {
		return state
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return localState{HistoryEnabled: true}
	}
	return state
}

func (a *App) saveLocalState() {
	if os.MkdirAll(a.dir, 0o700) != nil {
		return
	}
	data, err := json.Marshal(a.local)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(a.dir, "local-state-*.tmp")
	if err != nil {
		return
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if tmp.Chmod(0o600) != nil || tmp.Close() != nil {
		return
	}
	_ = os.Rename(name, filepath.Join(a.dir, "local-state.json"))
}

func (a *App) recordHistory(feed backend.Feed) {
	if !a.local.HistoryEnabled || feed.ID == "" {
		return
	}
	title := feed.Title
	if title == "" {
		title = truncateRunes(feed.Content, 80)
	}
	entry := historyEntry{FeedID: feed.ID, Title: title, Author: displayName(feed.Author.Name), AvatarURL: feed.Author.AvatarURL, ViewedAt: time.Now()}
	history := make([]historyEntry, 0, len(a.local.History)+1)
	history = append(history, entry)
	for _, item := range a.local.History {
		if item.FeedID != feed.ID {
			history = append(history, item)
		}
		if len(history) == 100 {
			break
		}
	}
	a.local.History = history
	a.history.Set(history)
	a.saveLocalState()
}

// keepSearches shows and stores h as the recent searches.
func (a *App) keepSearches(h []string) {
	a.searchHistory.Set(h)
	a.local.Searches = h
	a.saveLocalState()
}
