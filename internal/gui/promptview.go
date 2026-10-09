package gui

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// sheetWidth is the measure every sheet opens at: one width for the app's
// most consequential dialogs, so the question and the form are read at the
// same size.
const sheetWidth = 460

// sheetPanel is the surface every sheet is built on: one column holding the
// title block, the body, and the footer, on the ladder's spacing and the
// tray's corner.
func sheetPanel(c *ui.Context, p palette, fn func()) ui.Element {
	return ui.Column(c).
		Width(sheetWidth).
		Padding(space5).
		Gap(space4).
		Radius(radiusTray).
		Background(p.sheet).
		Border(1, p.border).
		Shadow(0, 18, 44, -8, ui.RGBA(12, 10, 30, 0.32)).
		Shadow(0, 2, 6, 0, ui.RGBA(12, 10, 30, 0.10)).
		Children(fn)
}

// sheetHeader is a sheet's title and the line that explains it.
func sheetHeader(c *ui.Context, p palette, text, explain string) ui.Element {
	return ui.Column(c).Gap(space1).Children(func() {
		chrome(c, p, text)
		if explain != "" {
			muted(c, p, explain)
		}
	})
}

// sheetFooter is the row a sheet ends with: the actions right-aligned, the
// secondary one first and the primary last.
func sheetFooter(c *ui.Context, fn func()) ui.Element {
	return ui.Row(c).FillWidth().Gap(space2).Justify(ui.End).Children(fn)
}

// promptSheet is the credential sheet the main window shows over its content.
func (a *App) promptSheet(c *ui.Context, p palette) {
	if !a.prompt.active() {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		sheetPanel(c, p, func() {
			sheetHeader(c, p, a.promptHeader(), a.promptSubtitle())
			a.promptBody(c, p)
			if a.prompt.err != "" {
				errorLine(c, p, a.prompt.err, a.prompt.errDetail)
			}
			sheetFooter(c, func() {
				if btnLarge(ui.Button(c, a.promptCancelLabel())).Clicked() {
					a.cancelPrompt()
				}
				if a.prompt.kind != promptCLI || a.prompt.done {
					text := "保存"
					if a.prompt.kind == promptCLI {
						text = "完成"
					}
					if btnLarge(ui.PrimaryButton(c, text)).Disabled(a.prompt.running).Clicked() {
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
		return "粘贴 API 密钥"
	case promptCookieImport:
		return "读取浏览器登录状态"
	case promptManualCookie:
		return "粘贴登录状态"
	case promptSessionFile:
		return "导入登录文件"
	case promptNewAccount:
		return "添加账号"
	case promptCLI:
		return "用官方工具登录"
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

// promptBody is the field or log each kind of sheet shows. Every sheet with
// fields lays them out as the settings window does: the label above its
// control, one step between rows.
func (a *App) promptBody(c *ui.Context, p palette) {
	switch a.prompt.kind {
	case promptAPIKey:
		hint := "API 密钥会写进 ~/.config/leeway/config.json，不打印、不上传。"
		if a.prompt.keepKey {
			hint = "已经有一份密钥：留空保持不变，填新的会覆盖。"
		}
		formRow(c, p, "API 密钥", hint, func() {
			ui.Column(c).FillWidth().Gap(space2).Children(func() {
				input := ui.TextInput(c, &a.prompt.key)
				if !a.prompt.reveal {
					input.Password()
				}
				input.AutoFocus()
				if input.Submitted() {
					a.submitPrompt()
				}
				ui.Checkbox(c, &a.prompt.reveal, "显示出来")
			})
		})

	case promptCookieImport:
		ui.Column(c).FillWidth().Gap(space3).Children(func() {
			formRow(c, p, "浏览器", "", func() {
				ui.TextInput(c, &a.prompt.browser).Placeholder("chrome / safari / edge / arc / brave")
			})
			formRow(c, p, "浏览器配置（可留空）",
				"leeway 从浏览器读出这个服务商的登录状态，只存在你自己的缓存目录。", func() {
					ui.TextInput(c, &a.prompt.profile).Placeholder("Default / Profile 1")
				})
		})

	case promptManualCookie:
		formRow(c, p, "登录状态",
			"浏览器读不到时用这个：从开发者工具复制 Cookie。这里显示的是明文，旁边有人或录屏时注意。",
			func() {
				ui.TextArea(c, &a.prompt.header).
					Placeholder("name=value; name2=value2（curl -b 或 Cookie: 片段也行）").
					AutoFocus()
			})

	case promptSessionFile:
		formRow(c, p, "登录文件",
			"leeway 把这份文件备份到自己的目录，之后切换账号不会动原文件。", func() {
				ui.Row(c).FillWidth().Gap(space2).Children(func() {
					ui.TextInput(c, &a.prompt.path).Grow(1)
					if btnLarge(ui.Button(c, "选择…")).Clicked() {
						a.pickSessionFile()
					}
				})
			})

	case promptNewAccount:
		formRow(c, p, "账号名",
			"只是个标签。之后可以单独给它登录一份，再用「启用」切换。", func() {
				input := ui.TextInput(c, &a.prompt.name).Placeholder("work / home / 邮箱")
				input.AutoFocus()
				if input.Submitted() {
					a.submitPrompt()
				}
			})

	case promptCLI:
		ui.Column(c).FillWidth().Gap(space3).Children(func() {
			// The line is only there while it has something to say, so a
			// failed login does not leave a gap above its own error.
			if a.prompt.running || (a.prompt.done && a.prompt.err == "") {
				ui.Row(c).Gap(space2).AlignItems(ui.Center).Children(func() {
					if a.prompt.running {
						ui.Spinner(c).Size(iconBody, iconBody)
						muted(c, p, "等待官方工具完成登录…")
					} else {
						body(c, p, "登录完成").TextColor(p.toneColor(toneOK))
					}
				})
			}
			log := ""
			if a.prompt.log != nil {
				log = a.prompt.log.String()
			}
			if strings.TrimSpace(log) == "" {
				log = "（官方工具还没有输出）"
			}
			ui.Scroll(c).
				FillWidth().
				Height(210).
				Padding(space3).
				Radius(radiusControl).
				Background(p.track).
				Children(func() {
					ui.Text(c, strings.TrimRight(log, "\n")).
						FontSize(sizeLabel).
						Font(p.mono).
						TextColor(p.t.TextMuted).
						Selectable()
				})
			unconfiguredLine(c, p, "登录在 leeway 自己的目录里进行，不会覆盖官方工具的登录。")
		})
	}
}

// pickSessionFile asks the system for a session file, off the main thread
// because the dialog blocks.
func (a *App) pickSessionFile() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:       "选择登录文件",
			DefaultPath: a.prompt.path,
			Filters:     []mygo.FileFilter{{Name: "登录文件", Extensions: []string{"json"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		chosen := paths[0]
		a.apply(func() { a.prompt.path = chosen })
	}()
}

// confirmSheet asks about one irreversible action, in the app's own voice:
// mygo's AlertDialog is too small to carry the reason for a deletion. It is
// the sheet skeleton without a body: the title, the reason under it, and the
// actions.
func (a *App) confirmSheet(c *ui.Context, p palette) {
	if !a.confirm.open {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		sheetPanel(c, p, func() {
			sheetHeader(c, p, a.confirm.title, a.confirm.message)
			sheetFooter(c, func() {
				if btnLarge(ui.Button(c, "取消")).Clicked() {
					a.confirm = confirm{}
					return
				}
				// The critical fill is a light red in the dark theme the way
				// the accent is a light violet, so its label takes the same
				// ink a primary button's does: white on it is 2.68:1 there.
				action := btnLarge(ui.Button(c, a.confirm.label)).
					Background(p.toneColor(toneCritical)).
					TextColor(p.t.AccentText)
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
