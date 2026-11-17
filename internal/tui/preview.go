package tui

import (
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// NewPreview starts at the language screen and opens only the injected temporary vault.
// The caller owns and removes the directory containing path when the session ends.
func NewPreview(path string, preferred Language, id *identity.Identity) Model {
	m := New(path, preferred)
	m.preview = true
	m.previewID = id
	m.start = spacesList
	return m
}

var previewItems = []struct{ name, value string }{
	{"example: email", "demo@example.invalid"},
	{"example: bank", "DEMO-ACCOUNT-0000"},
	{"example: github token", "fake-token-not-valid"},
	{"example: wifi", "sample-wifi-password"},
}

func (m *Model) startPreview() {
	id := m.previewID
	m.previewID = nil
	m.openVault(id)
	if m.vlt == nil {
		return
	}
	for _, item := range previewItems {
		if err := m.vlt.Put(item.name, []byte(item.value)); err != nil {
			m.failVault(err)
			return
		}
	}
	work, err := vault.NewScope(vaultDir(m.path), id, "Work")
	if err != nil {
		m.failVault(err)
		return
	}
	m.scopes = append(m.scopes, work)
	for _, item := range []struct{ name, value string }{{"example: meeting", "sample-agenda"}, {"example: support", "support@example.invalid"}} {
		if err := work.Put(item.name, []byte(item.value)); err != nil {
			m.failVault(err)
			return
		}
	}
	if err := m.sortSpaces(m.scopes[0]); err != nil {
		m.failVault(err)
		return
	}
	m.refreshSpaces()
}
