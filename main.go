package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"

	"aerobix/paths"
	"aerobix/provider"
	"aerobix/provider/mock"
	"aerobix/provider/strava"
	"aerobix/ui"
)

func main() {
	if err := paths.MigrateLegacy(); err != nil {
		log.Printf("aerobix: profile migration: %v", err)
	}
	dataProvider := buildProvider()

	program := tea.NewProgram(ui.NewModel(dataProvider), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		log.Fatal(err)
	}
}

func buildProvider() provider.DataProvider {
	p, err := strava.NewProvider()
	if err != nil {
		fallback := mock.NewProvider()
		return fallback
	}
	return &p
}
