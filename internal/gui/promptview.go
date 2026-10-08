package gui

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// promptSheet is the credential sheet the main window shows over its content.
func (a *App) promptSheet(c *ui.Context, p palette) {
	if !a.prompt.active() {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).
			Width(470).
			Padding(22).
			Gap(16).
			Radius(20).
			Background(p.sheet).
			Border(1, p.border).
			Shadow(0, 18, 44, -8, ui.RGBA(12, 10, 30, 0.32)).
			Shadow(0, 2, 6, 0, ui.RGBA(12, 10, 30, 0.10)).
			Children(func() {
				ui.Column(c).Gap(5).Children(func() {
					ui.Text(c, a.promptHeader()).
						FontSize(18).
						FontWeight(650).
						LetterSpacing(-0.3)
					if a.promptSubtitle() != "" {
						muted(c, p, a.promptSubtitle())
					}
				})
				a.promptBody(c, p)
				if a.prompt.err != "" {
					errorLine(c, p, a.prompt.err)
				}
				ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Padding(4, 0, 0, 0).Children(func() {
					if ui.Button(c, a.promptCancelLabel()).Padding(7, 14).Radius(9).FontSize(sizeBody).Clicked() {
						a.cancelPrompt()
					}
					if a.prompt.kind != promptCLI || a.prompt.done {
						label := "保存"
						if a.prompt.kind == promptCLI {
							label = "完成"
						}
						if ui.PrimaryButton(c, label).Padding(7, 16).Radius(9).FontSize(sizeBody).Disabled(a.prompt.running).Clicked() {
							a.submitPrompt()
						}
					}
				})
			})
	})
	if !open {
		a.cancelPrompt()
	}
}

// promptHeader names the sheet after what it does.
func (a *App) promptHeader() string {
	switch a.prompt.kind {
	case promptAPIKey:
		return "粘贴 API key"
	case promptCookieImport:
		return "从浏览器导入 cookie"
	case promptManualCookie:
		return "粘贴 cookie header"
	case promptSessionFile:
		return "导入会话文件"
	case promptNewAccount:
		return "添加账号"
	case promptCLI:
		return "用官方 CLI 登录"
	}
	return "登录"
}

// subtitle says which provider and account the sheet is about.
func (a *App) promptSubtitle() string {
	if a.prompt.provider == "" {
		return a.prompt.title
	}
	name := providerName(a.prompt.provider)
	if a.prompt.account != "" && a.prompt.account != "default" {
		return name + " · 账号 " + a.prompt.account
	}
	return name
}

func (a *App) promptCancelLabel() string {
	if a.prompt.kind == promptCLI {
		if a.prompt.running {
			return "停止"
		}
		return "关闭"
	}
	return "取消"
}

// cancelPrompt stops a running login and closes the sheet.
func (a *App) cancelPrompt() {
	if a.prompt.cancel != nil {
		a.prompt.cancel()
	}
	a.prompt.reset()
}

// promptBody is the field or log each kind of sheet shows.
func (a *App) promptBody(c *ui.Context, p palette) {
	switch a.prompt.kind {
	case promptAPIKey:
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			label(c, p, "API key")
			input := ui.TextInput(c, &a.prompt.key)
			if !a.prompt.reveal {
				input.Password()
			}
			input.AutoFocus()
			if input.Submitted() {
				a.submitPrompt()
			}
			ui.Checkbox(c, &a.prompt.reveal, "显示明文")
			hint := "key 会写进 ~/.config/leeway/config.json（0600），不打印、不上传。"
			if a.prompt.keepKey {
				hint = "已经有一把 key：留空就是保持不变，填新的会覆盖。"
			}
			unconfiguredLine(c, p, hint)
		})

	case promptCookieImport:
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			ui.Column(c).Gap(6).Children(func() {
				label(c, p, "浏览器")
				ui.TextInput(c, &a.prompt.browser).Placeholder("chrome / safari / edge / arc / brave")
			})
			ui.Column(c).Gap(6).Children(func() {
				label(c, p, "Profile（可留空）")
				ui.TextInput(c, &a.prompt.profile).Placeholder("Default / Profile 1")
			})
			unconfiguredLine(c, p, "leeway 从浏览器的 cookie 库里读出这个 provider 的登录态，只存在你自己的缓存目录。")
		})

	case promptManualCookie:
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			label(c, p, "cookie header")
			ui.TextArea(c, &a.prompt.header).Placeholder("name=value; name2=value2（curl -b 或 Cookie: 片段也行）").AutoFocus()
			unconfiguredLine(c, p, "适合浏览器导入不成功的场景：从开发者工具里复制 Cookie 请求头。这个框是明文，旁边有人或录屏时注意。")
		})

	case promptSessionFile:
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			label(c, p, "会话文件")
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.TextInput(c, &a.prompt.path).Grow(1)
				if ui.Button(c, "选择…").Clicked() {
					a.pickSessionFile()
				}
			})
			unconfiguredLine(c, p, "leeway 把这份文件快照到自己的会话目录，之后切号不会动原文件。")
		})

	case promptNewAccount:
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			label(c, p, "账号名")
			input := ui.TextInput(c, &a.prompt.name).Placeholder("work / home / 邮箱")
			input.AutoFocus()
			if input.Submitted() {
				a.submitPrompt()
			}
			unconfiguredLine(c, p, "只是个标签，之后可以给它单独登录一份凭据，再用「让本机用它」切换。")
		})

	case promptCLI:
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if a.prompt.running {
					ui.Spinner(c).Size(14, 14)
					muted(c, p, "等待官方 CLI 完成登录…")
				} else if a.prompt.done && a.prompt.err == "" {
					ui.Text(c, "登录完成").FontSize(sizeSmall).TextColor(p.toneColor(toneOK))
				}
			})
			log := ""
			if a.prompt.log != nil {
				log = a.prompt.log.String()
			}
			if strings.TrimSpace(log) == "" {
				log = "（官方 CLI 还没有输出）"
			}
			ui.Scroll(c).
				FillWidth().
				Height(210).
				Padding(12).
				Radius(10).
				Background(p.track).
				Children(func() {
					ui.Text(c, strings.TrimRight(log, "\n")).
						FontSize(sizeLabel).
						Font(p.mono).
						TextColor(p.t.TextMuted).
						Selectable()
				})
			unconfiguredLine(c, p, "登录在 leeway 自己的 HOME 里进行，不会覆盖你本机官方客户端的登录。")
		})
	}
}

// pickSessionFile asks the system for a session file, off the main thread
// because the dialog blocks.
func (a *App) pickSessionFile() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:       "选择会话文件",
			DefaultPath: a.prompt.path,
			Filters:     []mygo.FileFilter{{Name: "会话", Extensions: []string{"json"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		chosen := paths[0]
		a.apply(func() { a.prompt.path = chosen })
	}()
}

// confirmSheet asks about one irreversible action, in the app's own voice:
// mygo's AlertDialog is too small to carry the reason for a deletion.
func (a *App) confirmSheet(c *ui.Context, p palette) {
	if !a.confirm.open {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).
			Width(430).
			Padding(22).
			Gap(16).
			Radius(20).
			Background(p.sheet).
			Border(1, p.border).
			Shadow(0, 18, 44, -8, ui.RGBA(12, 10, 30, 0.32)).
			Shadow(0, 2, 6, 0, ui.RGBA(12, 10, 30, 0.10)).
			Children(func() {
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, a.confirm.title).
						FontSize(17).
						FontWeight(650).
						LetterSpacing(-0.3)
					ui.Text(c, a.confirm.message).
						FontSize(sizeBody).
						TextColor(p.t.TextMuted).
						LineHeight(1.5)
				})
				ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "取消").Padding(7, 14).Radius(9).FontSize(sizeBody).Clicked() {
						a.confirm = confirm{}
						return
					}
					action := ui.Button(c, a.confirm.label).
						Padding(7, 16).
						Radius(9).
						FontSize(sizeBody).
						Background(p.toneColor(toneCritical)).
						TextColor(ui.RGB(255, 255, 255))
					if action.Clicked() {
						chosen := a.confirm
						a.confirm = confirm{}
						a.runConfirm(chosen)
					}
				})
			})
	})
	if !open {
		a.confirm = confirm{}
	}
}
