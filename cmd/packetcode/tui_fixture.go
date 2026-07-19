package main

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/app"
)

func runTUIFixture(state string) error {
	model, err := app.NewTUIFixture(state)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(model).Run()
	return err
}
