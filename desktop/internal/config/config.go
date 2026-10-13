// Package config stores the receiver's pairing secret and settings.
package config

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

// File is the on-disk receiver configuration. It holds the pairing secret,
// so it is written with owner-only permissions and never logged.
type File struct {
	PairingSecret string `json:"pairingSecret"` // hex
	Name          string `json:"name,omitempty"`
	// Profile is the keyboard/mouse profile last used, so a plain
	// double-click of the Mac app resumes it.
	Profile string `json:"profile,omitempty"`

	path string
}

// DefaultPath returns the config path: $POCKETPAD_CONFIG, or receiver.json in
// the user's configuration directory.
func DefaultPath() (string, error) {
	if p := os.Getenv("POCKETPAD_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "PocketPad", "receiver.json"), nil
}

// Load reads the config at path, creating it with a fresh pairing secret
// if it does not exist.
func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		f := &File{path: path}
		if err := f.ResetSecret(); err != nil {
			return nil, err
		}
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	f := &File{path: path}
	if err := json.Unmarshal(b, f); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	if _, err := f.Secret(); err != nil {
		return nil, fmt.Errorf("config %s: %w (run \"pocketpad pair -reset\")", path, err)
	}
	return f, nil
}

// Secret returns the pairing-secret bytes.
func (f *File) Secret() ([]byte, error) {
	b, err := hex.DecodeString(f.PairingSecret)
	if err != nil || len(b) != protocol.PairingCodeBytes {
		return nil, errors.New("invalid pairing secret")
	}
	return b, nil
}

// ResetSecret generates a new pairing secret and saves the file. Phones
// paired with the old secret must pair again.
func (f *File) ResetSecret() error {
	s, err := protocol.NewPairingSecret()
	if err != nil {
		return err
	}
	f.PairingSecret = hex.EncodeToString(s)
	return f.Save()
}

// SetProfile remembers the keyboard/mouse profile and saves the file.
func (f *File) SetProfile(name string) error {
	f.Profile = name
	return f.Save()
}

// Save writes the file atomically with owner-only permissions.
func (f *File) Save() error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

// Path is where the file is stored.
func (f *File) Path() string { return f.path }
