// leeway — 余量·裕度: AI coding-provider usage, limits and resets.
//
// Pure CLI clone of CodexBar's provider probing core. JSON-first output for
// agent integration (Hermes etc.); stdout carries only data.
package main

import (
	"os"

	_ "leeway/internal/provider/codex"
	_ "leeway/internal/provider/cursor"
	_ "leeway/internal/provider/deepseek"
	_ "leeway/internal/provider/grok"
	_ "leeway/internal/provider/kimi"
	_ "leeway/internal/provider/opencodego"

	"leeway/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
