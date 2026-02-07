package main

import (
	"embed"
	"io/fs"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/api"
)

//go:embed dashboard/*
var dashboardFS embed.FS

//go:embed runners/*
var runnersFS embed.FS

func init() {
	// Dashboard
	_, err := fs.Stat(dashboardFS, "dashboard/index.html")
	if err == nil {
		api.DashboardFS = dashboardFS
		api.DashboardPrefix = "dashboard"
		api.HasDashboard = true
	}

	// Runners
	_, err = fs.Stat(runnersFS, "runners/select-runner")
	if err == nil {
		api.RunnersFS = runnersFS
		api.RunnersPrefix = "runners"
		api.HasRunners = true
	}
}
