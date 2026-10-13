package config

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadCreatesAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "receiver.json")
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s1, err := f.Secret()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", st.Mode())
		}
	}
	f2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := f2.Secret()
	if !bytes.Equal(s1, s2) {
		t.Fatal("secret changed on reload")
	}
	if err := f2.ResetSecret(); err != nil {
		t.Fatal(err)
	}
	f3, _ := Load(path)
	s3, _ := f3.Secret()
	if bytes.Equal(s1, s3) {
		t.Fatal("reset kept the secret")
	}
}

func TestLoadRejectsCorruptSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receiver.json")
	os.WriteFile(path, []byte(`{"pairingSecret":"abcd"}`), 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("accepted a short secret")
	}
}
