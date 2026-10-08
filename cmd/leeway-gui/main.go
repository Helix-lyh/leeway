// Command leeway-gui opens leeway in a window: the same probes, config and
// history as the CLI, drawn with github.com/egoist/mygo's native UI.
package main

import (
	"github.com/egoist/mygo"

	// Providers register themselves, as they do for the CLI.
	_ "leeway/internal/provider/codex"
	_ "leeway/internal/provider/commandcode"
	_ "leeway/internal/provider/cursor"
	_ "leeway/internal/provider/deepseek"
	_ "leeway/internal/provider/grok"
	_ "leeway/internal/provider/kimi"
	_ "leeway/internal/provider/opencodego"

	"leeway/internal/gui"
)

func main() {
	mygo.App.SetName("leeway")
	mygo.App.SetVersion(gui.Version)

	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	app := gui.NewApp()
	mygo.App.SetMenu(app.Menu())
	mygo.App.WhenReady(app.Open)
	mygo.App.OnSecondInstance(func([]string, string) { app.Focus() })

	if err := mygo.App.Run(); err != nil {
		panic(err)
	}
}
