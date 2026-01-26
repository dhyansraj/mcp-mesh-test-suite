package main

import (
	"embed"
	"io/fs"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/api"
)

//go:embed dashboard/*
var dashboardFS embed.FS

func init() {
	// Check if dashboard files are actually embedded by looking for index.html
	// (the embed directive requires the directory to exist at build time,
	// but we only enable dashboard serving if actual build output exists)
	_, err := fs.Stat(dashboardFS, "dashboard/index.html")
	if err == nil {
		api.DashboardFS = dashboardFS
		api.DashboardPrefix = "dashboard"
		api.HasDashboard = true
	}
}
