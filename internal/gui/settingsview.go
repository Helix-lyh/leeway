package gui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// settingsView is the settings window: the same app state as the main window,
// a second window of its own.
func (a *App) settingsView(c *ui.Context) {
	p := a.theme(c)
	a.settingsArrival.index = 0
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
	a.burnToast(c)
	tb := c.TitleBar()
	height := tb.Height
	if height < 54 {
		height = 54
	}
	left := float32(18)
	if tb.Left > 0 {
		left = tb.Left + 12
	}
	ui.Column(c).
		Fill().
		LinearGradient(ui.LinearGradient{From: p.canvasTop, To: p.canvas, Angle: 180, Oklab: true}).
		Children(func() {
			ui.Row(c).
				FillWidth().
				Height(height).
				Padding(0, 18, 0, left).
				Gap(12).
				AlignItems(ui.Center).
				DragWindow().
				Material(glass.Glass{}).
				Children(func() {
					ui.Text(c, "设置").
						FontSize(15).
						FontWeight(700).
						LetterSpacing(-0.2)
					ui.Box(c).Width(1).Height(18).Background(p.t.TextMuted.Alpha(0.28))
					muted(c, p, "窗口偏好写在 gui.json，与命令行的 config.json 分开")
					ui.Spacer(c)
					ui.Tabs(c, &a.settingsTab, "通用", "Provider", "数据")
				})
			ui.Scroll(c).Grow(1).FillWidth().Padding(24, 26, 30, 26).Children(func() {
				// The window's content arrives as one block: a settings
				// window has one thing to read, not a page of cards.
				a.settingsArrival.hold(c, a.now(), func() {
					ui.Column(c).FillWidth().Gap(18).Children(func() {
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
		})
}

// settingsGeneral is refresh, appearance and startup.
func (a *App) settingsGeneral(c *ui.Context, p palette) {
	card(c, p, func() {
		section(c, p, "刷新")
		labeledRow(c, p, "自动刷新", func() {
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
		unconfiguredLine(c, p, "自动刷新读一分钟内的缓存；手动刷新总是重新探测。")

		labeledRow(c, p, "探测超时（秒）", func() {
			secs := float64(a.settings.TimeoutSec)
			field := ui.NumberInput(c, &secs, 5, 300, 5)
			if field.Changed() {
				a.settings.TimeoutSec = int(secs)
				a.SaveSettings()
			}
		})
		unconfiguredLine(c, p, "单个 provider 的 HTTP 超时，等价于 LEEWAY_TIMEOUT。")
	})

	card(c, p, func() {
		section(c, p, "外观")
		labeledRow(c, p, "主题", func() { appearancePicker(c, p, a) })
		labeledRow(c, p, "打开窗口时探测", func() { switchRow(c, &a.settings.OnOpenFetch, a) })
		labeledRow(c, p, "探测失败时发通知", func() { switchRow(c, &a.settings.NotifyOnError, a) })
		if mygo.App.IsReady() && !mygo.NotificationsSupported() {
			unconfiguredLine(c, p, "这个环境不支持系统通知（macOS 只在打包安装后的构建里允许）。")
		}
	})

	card(c, p, func() {
		section(c, p, "启动")
		labeledRow(c, p, "开机自动启动", func() {
			on := mygo.App.IsReady() && mygo.App.OpenAtLogin()
			if ui.Switch(c, &on).Changed() && mygo.App.IsReady() {
				if err := mygo.App.SetOpenAtLogin(on); err != nil {
					a.toast = "开机自启设置失败：" + err.Error()
				}
			}
		})
		unconfiguredLine(c, p, "需要打包后的 .app（macOS 13+）；未打包运行时这个开关会失败并提示原因。")
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
	return ui.Row(c).Gap(10).Children(func() {
		for _, o := range options {
			opt := o
			selected := a.settings.Theme == opt.value
			ui.Column(c).Gap(6).AlignItems(ui.Center).Children(func() {
				swatch := ui.Box(c).
					Size(62, 40).
					Radius(10).
					Label("主题："+opt.label).
					Border(1, p.border).
					Draw(func(pt *ui.Painter, r ui.Rect) {
						canvas, card, track, accent := miniWindow(opt.dark)
						if opt.split {
							// Auto is the two themes it moves between.
							pt.FillGradient(r, ui.LinearGradient{
								From: ui.Hex("#ececf2"), To: ui.Hex("#131318"),
								Angle: 90, Oklab: true,
							}, 10)
							card, track = ui.RGBA(255, 255, 255, 0.55), ui.RGBA(0, 0, 0, 0.12)
						} else {
							pt.Fill(r, canvas, 10)
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
				if selected {
					ui.Text(c, opt.label).FontSize(sizeLabel).FontWeight(600).TextColor(p.t.Text)
				} else {
					label(c, p, opt.label)
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

// switchRow binds a Switch to a settings field and saves on change.
func switchRow(c *ui.Context, field *bool, a *App) ui.Element {
	on := *field
	sw := ui.Switch(c, &on)
	if sw.Changed() {
		*field = on
		a.SaveSettings()
	}
	return sw
}

// settingsProviders lists every provider with the switch that enables it.
func (a *App) settingsProviders(c *ui.Context, p palette) {
	card(c, p, func() {
		section(c, p, "启用的 provider")
		for _, v := range a.providers() {
			view := v
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				on := view.Enabled
				if ui.Switch(c, &on).Changed() {
					a.Enable(view.ID, on)
				}
				mark(c, p, view.ID, view.DisplayName, 26, "")
				ui.Text(c, view.DisplayName).FontSize(sizeBody).Grow(1).SingleLine()
				pill(c, p, statusLabel(view.Status, view.firstCred()), statusTone(view.Status))
				if ui.Button(c, "详情").Padding(5, 10).Radius(7).FontSize(sizeSmall).Clicked() {
					a.navigate("provider:" + view.ID)
					if a.win != nil && !a.win.IsDestroyed() {
						a.win.Show()
						a.win.Focus()
					}
				}
			})
		}
		unconfiguredLine(c, p, "停用的 provider 不参与探测。登录、切号和退出在各自的详情页里。")
	})
}

// settingsData is where leeway keeps its files, and what can be done to them.
func (a *App) settingsData(c *ui.Context, p palette) {
	card(c, p, func() {
		section(c, p, "历史")
		labeledRow(c, p, "每个窗口保留的探测次数", func() {
			n := float64(a.settings.HistoryN)
			field := ui.NumberInput(c, &n, 5, 200, 5)
			if field.Changed() {
				a.settings.HistoryN = int(n)
				a.SaveSettings()
				a.loadHistory()
			}
		})
		unconfiguredLine(c, p, "影响历史页的点阵，也是 CLI 里 usage --last 的默认次数。")
	})

	card(c, p, func() {
		section(c, p, "文件")
		for _, f := range []struct{ label, path string }{
			{"配置", configPathText()},
			{"缓存", cacheDirText()},
			{"会话", sessionDirText()},
			{"历史", historyPathText()},
		} {
			file := f
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				label(c, p, file.label).Width(40)
				path := ui.Text(c, shortenPath(file.path)).
					FontSize(sizeSmall).
					Font(p.mono).
					Grow(1).
					SingleLine().
					Ellipsis("…").
					Tooltip(file.path + "\n点击复制路径")
				if path.Clicked() {
					mygo.Clipboard.WriteText(file.path)
					a.toast = "已复制路径"
				}
				if ui.Button(c, "打开").Padding(5, 10).Radius(7).FontSize(sizeSmall).Clicked() {
					mygo.Shell.ShowItemInFolder(file.path)
				}
			})
		}
	})

	card(c, p, func() {
		section(c, p, "维护")
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if ui.Button(c, "复制当前 JSON").Clicked() {
				mygo.Clipboard.WriteText(a.CopyJSON())
				a.toast = "已复制 JSON"
			}
			if ui.Button(c, "清除 cookie 缓存").Clicked() {
				a.askClearCache()
			}
			if ui.Button(c, "立刻刷新").Disabled(a.fetching).Clicked() {
				a.Refresh(true)
			}
		})
		unconfiguredLine(c, p, "清除缓存只让下一次探测重新读一遍浏览器 cookie，不动 config.json 里的密钥。")
	})

	card(c, p, func() {
		section(c, p, "关于")
		keyValue(c, p, "版本", "leeway "+version)
		keyValue(c, p, "界面", "go "+goVersion())
	})
}
