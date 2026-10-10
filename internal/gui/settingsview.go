package gui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"log"
)

// formRow is a settings row: the label above its control, as docs/GUI.md asks
// of every form in the window, then the line that explains it. One step
// between the label and its control, one between the control and the line
// under it; the card's own gap is what separates two rows.
func formRow(c *ui.Context, p palette, name, hint string, control func()) ui.Element {
	return ui.Column(c).FillWidth().Gap(space2).Children(func() {
		ui.Column(c).FillWidth().Gap(space1).Children(func() {
			label(c, p, name)
			control()
		})
		if hint != "" {
			unconfiguredLine(c, p, hint)
		}
	})
}

// toggleRow is the exception the rule allows: a switch is a state rather than
// a value, so its name and the switch share a line, as every settings list on
// the platform reads. The name is a row's own title rather than a field's
// caption — label() is for the caption of a control that carries a value, and
// a switch row carries none.
func toggleRow(c *ui.Context, p palette, name string, control func(name string)) ui.Element {
	return ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(space3).Children(func() {
		body(c, p, name)
		ui.Box(c).Grow(1)
		control(name)
	})
}

// settingsView is the settings window: the same app state as the main window,
// a second window of its own.
// settingsSubtitle is the settings window's one line of context.
const settingsSubtitle = "窗口的设置存在 gui.json，与命令行的 config.json 分开"

func (a *App) settingsView(c *ui.Context) {
	p := a.theme(c)
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
	a.burnToast(c)
	tb := c.TitleBar()
	left := float32(space4)
	if tb.Left > 0 {
		left = tb.Left + space3
	}
	ui.Column(c).
		Fill().
		LinearGradient(ui.LinearGradient{From: p.canvasTop, To: p.canvas, Angle: 180, Oklab: true}).
		Children(func() {
			ui.Row(c).
				FillWidth().
				Height(a.titleBarHeight(c)).
				Padding(0, space4, 0, left).
				Gap(space3).
				AlignItems(ui.Center).
				DragWindow().
				Material(glass.Glass{}).
				Children(func() {
					// The title is the window's name: it never gives up room.
					// The line beside it is the only thing that may, and it
					// gives it up on one line rather than wrapping the bar.
					chrome(c, p, "设置").Shrink(0)
					chromeRule(c, p)
					// The one line in the window that has to give way when the
					// system text is large: it says its whole sentence as a
					// tooltip, so the truncation is never the end of it.
					muted(c, p, settingsSubtitle).
						SingleLine().
						Ellipsis("…").
						Tooltip(settingsSubtitle)
					ui.Spacer(c)
					ui.Tabs(c, &a.settingsTab, "通用", "服务商", "数据")
				})
			ui.Scroll(c).Grow(1).FillWidth().Padding(space5, space5, space6, space5).Children(func() {
				ui.Column(c).FillWidth().Gap(space4).Children(func() {
					switch a.settingsTab {
					case 1:
						a.settingsProviders(c, p)
					case 2:
						a.settingsData(c, p)
					default:
						a.settingsGeneral(c, p)
					}
				})
			})
		})
}

// settingsGeneral is refresh, appearance and startup.
func (a *App) settingsGeneral(c *ui.Context, p palette) {
	card(c, p, func() {
		section(c, p, "刷新")
		formRow(c, p, "自动刷新", "自动刷新用 1 分钟内的结果；手动刷新一定会重新查询。", func() {
			labels := make([]string, len(refreshOptions))
			for i, o := range refreshOptions {
				labels[i] = o.label
			}
			choice := refreshLabel(a.settings.IntervalSec)
			if ui.Select(c, &choice, labels).Width(190).Changed() {
				for _, o := range refreshOptions {
					if o.label == choice {
						a.settings.IntervalSec = o.sec
						a.SaveSettings()
					}
				}
			}
		})
		formRow(c, p, "查询超时（秒）", "单个服务商的查询超时，等同于 LEEWAY_TIMEOUT。", func() {
			secs := float64(a.settings.TimeoutSec)
			field := ui.NumberInput(c, &secs, 5, 300, 5)
			if field.Changed() {
				a.settings.TimeoutSec = int(secs)
				a.SaveSettings()
			}
		})
	})

	card(c, p, func() {
		section(c, p, "外观")
		formRow(c, p, "主题", "", func() { appearancePicker(c, p, a) })
		toggleRow(c, p, "打开窗口就查询", func(name string) { switchRow(c, &a.settings.OnOpenFetch, a, name) })
		toggleRow(c, p, "查询失败时通知我", func(name string) { switchRow(c, &a.settings.NotifyOnError, a, name) })
		if mygo.App.IsReady() && !mygo.NotificationsSupported() {
			unconfiguredLine(c, p, "这个环境不支持系统通知（只在安装后的 .app 里可用）。")
		}
	})

	card(c, p, func() {
		section(c, p, "启动")
		toggleRow(c, p, "开机自启", func(name string) {
			on := mygo.App.IsReady() && mygo.App.OpenAtLogin()
			if ui.Switch(c, &on).Label(name).Changed() && mygo.App.IsReady() {
				if err := mygo.App.SetOpenAtLogin(on); err != nil {
					log.Printf("窗口：设置开机自启失败：%v", err)
					a.toast, _ = failureText("开机自启设置失败", err.Error())
				}
			}
		})
		unconfiguredLine(c, p, "需要安装后的 .app（macOS 13 以上）。")
	})
}

// appearancePicker is the theme row: three little windows, each drawn in the
// theme it offers, so the choice is made by looking instead of by reading.
// "Follow the system" is drawn as the two themes it moves between.
func appearancePicker(c *ui.Context, p palette, a *App) ui.Element {
	options := []struct {
		value, label string
		dark         bool
		split        bool
	}{
		{"system", "跟随系统", p.dark, true},
		{"light", "浅色", false, false},
		{"dark", "深色", true, false},
	}
	return ui.Row(c).Gap(space2).Children(func() {
		for _, o := range options {
			opt := o
			selected := a.settings.Theme == opt.value
			ui.Column(c).Gap(space1).AlignItems(ui.Center).Children(func() {
				swatch := ui.Box(c).
					Size(62, 40).
					Radius(radiusControl).
					Label("主题："+opt.label).
					Border(1, p.border).
					Draw(func(pt *ui.Painter, r ui.Rect) {
						canvas, card, track, accent := miniWindow(opt.dark)
						if opt.split {
							// Auto is the two themes it moves between.
							pt.FillGradient(r, ui.LinearGradient{
								From: ui.Hex("#ececf2"), To: ui.Hex("#131318"),
								Angle: 90, Oklab: true,
							}, radiusControl)
							card, track = ui.RGBA(255, 255, 255, 0.55), ui.RGBA(0, 0, 0, 0.12)
						} else {
							pt.Fill(r, canvas, radiusControl)
						}
						panel := ui.Rect{X: r.X + 7, Y: r.Y + 7, W: r.W - 14, H: r.H - 14}
						pt.Fill(panel, card, 5)
						bar := ui.Rect{X: panel.X + 5, Y: panel.Y + panel.H/2 - 2, W: panel.W - 10, H: 4}
						pt.Fill(bar, track, 2)
						pt.Fill(ui.Rect{X: bar.X, Y: bar.Y, W: bar.W * 0.62, H: bar.H}, accent, 2)
					})
				if selected {
					swatch.Border(2, p.accent)
				}
				if swatch.Clicked() && !selected {
					a.settings.Theme = opt.value
					a.SaveSettings()
				}
				name := label(c, p, opt.label)
				if selected {
					name.TextColor(p.t.Text)
				}
			})
		}
	})
}

// miniWindow is the palette a preview swatch draws with: the real theme,
// pushed a little for contrast, because at this size subtlety is invisible.
func miniWindow(dark bool) (canvas, card, track, accent ui.Color) {
	p := paletteOf(&ui.Theme{Dark: dark})
	if dark {
		return ui.Hex("#131318"), ui.RGBA(255, 255, 255, 0.16), ui.RGBA(255, 255, 255, 0.22), p.accent
	}
	return ui.Hex("#ececf2"), ui.RGBA(255, 255, 255, 0.96), ui.RGBA(0, 0, 0, 0.10), p.accent
}

// switchRow binds a Switch to a settings field and saves on change. The switch
// carries the row's name as its own label: the words beside it are a separate
// element, so without it assistive technology reads out a switch with no name.
func switchRow(c *ui.Context, field *bool, a *App, name string) ui.Element {
	on := *field
	sw := ui.Switch(c, &on).Label(name)
	if sw.Changed() {
		*field = on
		a.SaveSettings()
	}
	return sw
}

// settingsProviders lists every provider with the switch that enables it.
// The rows are a list, not a form: the mark and the name are what the eye
// scans, so they lead, and the state and the actions trail — the switch last,
// where the platform puts it, rather than a column of switches in front of
// the names.
func (a *App) settingsProviders(c *ui.Context, p palette) {
	card(c, p, func() {
		section(c, p, "全部服务商")
		for i, v := range a.providers() {
			view := v
			if i > 0 {
				hairline(c, p)
			}
			ui.Row(c).FillWidth().Gap(space3).AlignItems(ui.Center).Children(func() {
				mark(c, p, view.ID, view.DisplayName, 26, "")
				body(c, p, view.DisplayName).Grow(1).SingleLine()
				pill(c, p, statusLabel(view.Status, view.firstCred()), statusTone(view.Status))
				if btnSmall(ui.Button(c, "详情")).Clicked() {
					a.navigate("provider:" + view.ID)
					if a.win != nil && !a.win.IsDestroyed() {
						a.win.Show()
						a.win.Focus()
					}
				}
				on := view.Enabled
				if ui.Switch(c, &on).Label("添加 " + view.DisplayName).Changed() {
					a.Enable(view.ID, on)
				}
			})
		}
		unconfiguredLine(c, p, "已移除的服务商不会被查询。登录、切换账号和移除，都在它的详情页里。")
	})
}

// settingsData is where leeway keeps its files, and what can be done to them.
func (a *App) settingsData(c *ui.Context, p palette) {
	card(c, p, func() {
		section(c, p, "历史")
		formRow(c, p, "每个额度周期保留的查询次数", "影响历史页的走势图，也是命令行 usage --last 的默认值。", func() {
			n := float64(a.settings.HistoryN)
			field := ui.NumberInput(c, &n, 5, 200, 5)
			if field.Changed() {
				a.settings.HistoryN = int(n)
				a.SaveSettings()
				a.loadHistory()
			}
		})
	})

	card(c, p, func() {
		section(c, p, "文件")
		for i, f := range []struct{ label, path string }{
			{"配置", configPathText()},
			{"缓存", cacheDirText()},
			{"登录文件", sessionDirText()},
			{"历史", historyPathText()},
		} {
			file := f
			if i > 0 {
				hairline(c, p)
			}
			ui.Row(c).FillWidth().Gap(space3).AlignItems(ui.Center).Children(func() {
				label(c, p, file.label).Width(labelColumn)
				path := body(c, p, shortenPath(file.path)).
					Font(p.mono).
					Grow(1).
					SingleLine().
					Ellipsis("…").
					Tooltip(file.path + "\n点击复制路径")
				if path.Clicked() {
					mygo.Clipboard.WriteText(file.path)
					a.toast = "已复制路径"
				}
				if btnSmall(ui.Button(c, "打开")).Clicked() {
					mygo.Shell.ShowItemInFolder(file.path)
				}
			})
		}
	})

	card(c, p, func() {
		section(c, p, "维护")
		ui.Row(c).Gap(space2).Wrap().Children(func() {
			if btnSmall(ui.Button(c, "复制数据")).Clicked() {
				mygo.Clipboard.WriteText(a.CopyJSON())
				a.toast = "已复制"
			}
			if btnSmall(ui.Button(c, "重新读取浏览器登录状态")).Clicked() {
				a.askClearCache()
			}
			if btnSmall(ui.Button(c, "刷新")).Disabled(a.fetching).Clicked() {
				a.Refresh(true)
			}
		})
		unconfiguredLine(c, p, "只让下一次查询重新读一遍浏览器登录状态，不动 config.json 里的密钥。")
	})

	card(c, p, func() {
		section(c, p, "关于")
		keyValue(c, p, "版本", "leeway "+version)
		keyValue(c, p, "运行环境", "go "+goVersion())
	})
}
