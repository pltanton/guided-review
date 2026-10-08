package publish

import (
	"embed"
	"fmt"

	"github.com/pltanton/guided-review/internal/state"
)

//go:embed gitlab.sh github.sh
var scripts embed.FS

func Script(provider string) ([]byte, error) {
	name := "gitlab.sh"
	if provider == state.ProviderGitHub {
		name = "github.sh"
	}
	data, err := scripts.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("publish script for %s: %w", provider, err)
	}
	return data, nil
}

func Tool(provider string) string {
	if provider == state.ProviderGitHub {
		return "gh"
	}
	return "glab"
}
