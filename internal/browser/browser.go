package browser

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/andrey-losikhin/zer0-waymarks/internal/protocol"
)

var (
	ErrBrowserNotAllowed = errors.New("browser not allowed")
	ErrProfileNotAllowed = errors.New("profile not allowed")
)

type Profile struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Args []string `json:"args,omitempty"`
}
type Browser struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Executable string    `json:"executable,omitempty"`
	Args       []string  `json:"args,omitempty"`
	Profiles   []Profile `json:"profiles"`
}
type Config struct {
	DefaultBrowserID string    `json:"default_browser_id,omitempty"`
	Browsers         []Browser `json:"browsers"`
}

var knownBrowsers = []struct {
	ID, Name string
	Commands []string
}{
	{"helium", "Helium", []string{"helium-browser", "helium"}},
	{"firefox", "Firefox", []string{"firefox"}},
	{"zen", "Zen Browser", []string{"zen-browser", "zen"}},
	{"librewolf", "LibreWolf", []string{"librewolf"}},
	{"floorp", "Floorp", []string{"floorp"}},
	{"waterfox", "Waterfox", []string{"waterfox"}},
	{"chrome", "Google Chrome", []string{"google-chrome-stable", "google-chrome"}},
	{"chromium", "Chromium", []string{"chromium", "chromium-browser"}},
	{"brave", "Brave", []string{"brave", "brave-browser"}},
	{"vivaldi", "Vivaldi", []string{"vivaldi-stable", "vivaldi"}},
	{"opera", "Opera", []string{"opera"}},
	{"edge", "Microsoft Edge", []string{"microsoft-edge-stable", "microsoft-edge"}},
	{"thorium", "Thorium", []string{"thorium-browser", "thorium"}},
}

func Load(path string) (Config, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			config := Config{}
			config.Discover()
			return config, nil
		}
		return Config{}, err
	}
	defer file.Close()
	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return Config{}, err
	}
	config.Discover()
	return config, nil
}

func (config *Config) Discover() {
	knownIDs := make(map[string]bool, len(config.Browsers))
	for _, item := range config.Browsers {
		knownIDs[item.ID] = true
	}
	for _, candidate := range knownBrowsers {
		if knownIDs[candidate.ID] {
			continue
		}
		for _, command := range candidate.Commands {
			executable, err := exec.LookPath(command)
			if err == nil {
				config.Browsers = append(config.Browsers, Browser{ID: candidate.ID, Name: candidate.Name, Executable: executable, Profiles: []Profile{}})
				knownIDs[candidate.ID] = true
				break
			}
		}
	}
	if config.DefaultBrowserID == "" && len(config.Browsers) > 0 {
		config.DefaultBrowserID = config.Browsers[0].ID
	}
}

func (config Config) SetDefault(id string) error {
	for _, item := range config.Browsers {
		if item.ID == id {
			return nil
		}
	}
	return ErrBrowserNotAllowed
}

func (config Config) Save(path string) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".config-*.json")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func (config Config) Public() []Browser {
	result := make([]Browser, len(config.Browsers))
	for index, browser := range config.Browsers {
		result[index] = Browser{ID: browser.ID, Name: browser.Name, Profiles: []Profile{}}
		for _, profile := range browser.Profiles {
			result[index].Profiles = append(result[index].Profiles, Profile{ID: profile.ID, Name: profile.Name})
		}
	}
	return result
}

func (config Config) Command(browserID, profileID, rawURL string) (*exec.Cmd, error) {
	if err := protocol.ValidateURL(rawURL); err != nil {
		return nil, err
	}
	if browserID == "" {
		browserID = config.DefaultBrowserID
	}
	for _, candidate := range config.Browsers {
		if candidate.ID != browserID {
			continue
		}
		args := append([]string{}, candidate.Args...)
		if profileID != "" {
			found := false
			for _, profile := range candidate.Profiles {
				if profile.ID == profileID {
					args = append(args, profile.Args...)
					found = true
					break
				}
			}
			if !found {
				return nil, ErrProfileNotAllowed
			}
		}
		args = append(args, rawURL)
		return exec.Command(candidate.Executable, args...), nil
	}
	return nil, ErrBrowserNotAllowed
}
