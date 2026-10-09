package gui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// startGuide is the window's first run, when no provider has answered yet: a
// row per provider saying what leeway needs from it, whether this machine
// already has that, and the one action that gets it going. It replaces the
// list of providers needing a hand, because on a first run they are the same
// seven.
func (a *App) startGuide(c *ui.Context, p palette) ui.Element {
	return card(c, p, func() {
		section(c, p, "开始使用")
		ui.Text(c, "leeway 不替你登录，也不碰你的密码。它只用你已有的登录信息，去问服务商还剩多少额度。").
			FontSize(sizeChrome).
			LineHeight(1.55).
			MaxWidth(620)
		hairline(c, p)
		for i, v := range a.providers() {
			if i > 0 {
				hairline(c, p)
			}
			a.startRow(c, p, v)
		}
		muted(c, p, "配好一个就能用：点「刷新」或按 ⌘R。命令行同样可用：leeway usage")
	})
}

// startRow is one provider on the guide.
func (a *App) startRow(c *ui.Context, p palette, v providerView) {
	cred := ""
	for _, av := range v.Accounts {
		if av.Cred != "" {
			cred = av.Cred
			break
		}
	}
	line := providerHint(v.ID)
	switch {
	case cred != "":
		line = credSentence(cred)
	case !v.Enabled:
		line = "已移除，不会被查询。"
	}
	ui.Row(c).FillWidth().PaddingY(11).Gap(12).AlignItems(ui.Center).Children(func() {
		mark(c, p, v.ID, v.DisplayName, 32, v.Status)
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, v.DisplayName).
					Role(ui.RoleHeading).
					Level(2).
					FontSize(sizeBody).
					FontWeight(600).
					SingleLine()
				if cred != "" {
					pill(c, p, "已找到登录信息", toneOK)
				}
			})
			muted(c, p, line)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			switch {
			case !v.Enabled:
				if btnSmall(ui.Button(c, "添加")).Clicked() {
					a.Enable(v.ID, true)
				}
			case cred != "":
				if btnSmall(ui.Button(c, "刷新")).Clicked() {
					a.Refresh(true)
				}
			default:
				if btnSmall(ui.Button(c, startAction(v.ID))).Clicked() {
					method := loginMethods(v.ID)[0]
					method.open(a, v.ID, defaultAccount(v))
				}
			}
			if btnSmall(ui.Button(c, "详情")).Clicked() {
				a.navigate("provider:" + v.ID)
			}
		})
	})
}

// startAction is the button on a provider's row: the first of the ways
// `leeway login` offers, without the ellipsis of a menu item.
func startAction(providerID string) string {
	return strings.TrimSuffix(loginMethods(providerID)[0].label, "…")
}
