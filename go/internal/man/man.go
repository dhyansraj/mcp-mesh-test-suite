// Package man provides documentation pages accessible via `tsuite man <topic>`.
package man

import (
	"embed"
	"strings"
)

//go:embed content/*.md
var contentFS embed.FS

// ManPage represents a documentation topic.
type ManPage struct {
	Name        string
	Title       string
	Description string
	Aliases     []string
}

// GetContent reads and returns the markdown content for this page.
func (p *ManPage) GetContent() (string, error) {
	data, err := contentFS.ReadFile("content/" + p.Name + ".md")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Registry of all man pages
var Pages = map[string]*ManPage{
	"quickstart": {
		Name:        "quickstart",
		Title:       "Quick Start",
		Description: "Get started with tsuite in minutes",
		Aliases:     []string{"quick", "start"},
	},
	"suites": {
		Name:        "suites",
		Title:       "Test Suites",
		Description: "Suite structure and config.yaml",
		Aliases:     []string{"suite", "config"},
	},
	"usecases": {
		Name:        "usecases",
		Title:       "Use Cases",
		Description: "Organizing tests into use cases",
		Aliases:     []string{"uc", "usecase"},
	},
	"testcases": {
		Name:        "testcases",
		Title:       "Test Cases",
		Description: "Test case structure and test.yaml",
		Aliases:     []string{"tc", "testcase", "test"},
	},
	"handlers": {
		Name:        "handlers",
		Title:       "Handlers",
		Description: "Built-in handlers (shell, http, wait, etc.)",
		Aliases:     []string{"handler"},
	},
	"routines": {
		Name:        "routines",
		Title:       "Routines",
		Description: "Reusable test routines",
		Aliases:     []string{"routine"},
	},
	"assertions": {
		Name:        "assertions",
		Title:       "Assertions",
		Description: "Assertion syntax and expressions",
		Aliases:     []string{"assert", "assertion"},
	},
	"artifacts": {
		Name:        "artifacts",
		Title:       "Artifacts",
		Description: "Test artifacts and file mounting",
		Aliases:     []string{"artifact", "files"},
	},
	"variables": {
		Name:        "variables",
		Title:       "Variables",
		Description: "Variable interpolation syntax",
		Aliases:     []string{"vars", "interpolate", "interpolation"},
	},
	"docker": {
		Name:        "docker",
		Title:       "Docker Mode",
		Description: "Container isolation and Docker execution",
		Aliases:     []string{"container", "containers"},
	},
	"api": {
		Name:        "api",
		Title:       "API & Dashboard",
		Description: "REST API server and web dashboard",
		Aliases:     []string{"server", "dashboard", "ui"},
	},
	"scaffold": {
		Name:        "scaffold",
		Title:       "Scaffold",
		Description: "Generate test cases from agent directories",
		Aliases:     []string{"generate", "gen"},
	},
}

// aliasMap maps aliases to canonical page names
var aliasMap map[string]string

func init() {
	aliasMap = make(map[string]string)
	for name, page := range Pages {
		aliasMap[name] = name
		for _, alias := range page.Aliases {
			aliasMap[alias] = name
		}
	}
}

// GetPage returns a man page by name or alias.
func GetPage(name string) *ManPage {
	canonical, ok := aliasMap[strings.ToLower(name)]
	if !ok {
		return nil
	}
	return Pages[canonical]
}

// ListPages returns all available man pages in a consistent order.
func ListPages() []*ManPage {
	// Return in a sensible order
	order := []string{
		"quickstart",
		"suites",
		"usecases",
		"testcases",
		"handlers",
		"routines",
		"assertions",
		"artifacts",
		"variables",
		"docker",
		"api",
		"scaffold",
	}

	pages := make([]*ManPage, 0, len(order))
	for _, name := range order {
		if page, ok := Pages[name]; ok {
			pages = append(pages, page)
		}
	}
	return pages
}
