package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"

	"aerobix/provider"
	"aerobix/provider/mock"
	"aerobix/provider/strava"
	"aerobix/ui"
)

func main() {
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
