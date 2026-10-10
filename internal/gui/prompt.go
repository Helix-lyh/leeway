package gui

import (
	"context"
	"fmt"
	"strings"
)

type promptKind int

const (
	promptNone promptKind = iota
	promptAPIKey
	promptCookieImport
	promptManualCookie
	promptSessionFile
	promptNewAccount
	promptCLI
)

// prompt is the login/credential sheet the window shows over the content.
type prompt struct {
	kind     promptKind
	provider string
	account  string
	title    string

	key     string
	keepKey bool // an empty field keeps the stored key instead of clearing it
	header  string
	path    string
	name    string
	browser string
	profile string
	reveal  bool
	err     string
	// errDetail is the provider's or the library's own words for the failure.
	// It is never the sentence a reader reads: it hangs off the line as a
	// tooltip, the same rule the page's errorLine follows.
	errDetail string

	running bool
	done    bool
	cancel  context.CancelFunc
	log     *loginLog
}

func (p *prompt) active() bool { return p.kind != promptNone }

func (p *prompt) reset() { *p = prompt{} }

// confirm is an alert dialog asking about one irreversible action.
type confirm struct {
	open     bool
	title    string
	message  string
	label    string
	kind     string
	provider string
	account  string
}

// --- opening each sheet ----------------------------------------------------

func (a *App) openAPIKey(providerID, account string) {
	hasKey := false
	if a.cfg != nil {
		if acc, ok := a.cfg.Account(providerID, account); ok {
			hasKey = acc.APIKey != ""
		}
	}
	a.prompt.reset()
	a.prompt.kind = promptAPIKey
	a.prompt.provider = providerID
	a.prompt.account = account
	a.prompt.title = "登录 " + providerID
	// The stored key is never put in the field: the window shows where a
	// credential comes from, not the credential. An empty field keeps it.
	a.prompt.keepKey = hasKey
}

func (a *App) openCookieImport(providerID, account string) {
	a.prompt.reset()
	a.prompt.kind = promptCookieImport
	a.prompt.provider = providerID
	a.prompt.account = account
	a.prompt.title = "导入 " + providerID + " 的浏览器登录状态"
	a.prompt.browser = "chrome"
	if a.cfg != nil {
		if acc, ok := a.cfg.Account(providerID, account); ok {
			a.prompt.profile = acc.Profile
		}
	}
}

func (a *App) openManualCookie(providerID, account string) {
	a.prompt.reset()
	a.prompt.kind = promptManualCookie
	a.prompt.provider = providerID
	a.prompt.account = account
	a.prompt.title = "粘贴 " + providerID + " 的登录状态"
}

func (a *App) openSessionFile(providerID, account string) {
	a.prompt.reset()
	a.prompt.kind = promptSessionFile
	a.prompt.provider = providerID
	a.prompt.account = account
	a.prompt.title = "导入 " + providerID + " 的登录文件"
	a.prompt.path = defaultSessionPath(providerID)
}

func defaultSessionPath(providerID string) string {
	switch providerID {
	case "grok":
		return "~/.grok/auth.json"
	case "kimi":
		return "~/.kimi-code/credentials/kimi-code.json"
	case "codex":
		return "~/.codex/auth.json"
	case "cursor":
		return "~/.cursor/auth.json"
	}
	return ""
}

func (a *App) openNewAccount(providerID string) {
	a.prompt.reset()
	a.prompt.kind = promptNewAccount
	a.prompt.provider = providerID
	a.prompt.title = "给 " + providerID + " 加一个账号"
}

// openCLI starts the provider's official CLI sign-in and shows what it prints.
func (a *App) openCLI(providerID, account string) {
	a.prompt.reset()
	a.prompt.kind = promptCLI
	a.prompt.provider = providerID
	a.prompt.account = account
	a.prompt.title = providerID + " 官方登录"
	a.prompt.log = &loginLog{}
	a.runCLI()
}

func (a *App) runCLI() {
	ctx, cancel := context.WithCancel(context.Background())
	a.prompt.cancel = cancel
	a.prompt.running = true
	a.prompt.done = false
	a.prompt.err = ""
	a.prompt.log.reset()

	providerID, account, log := a.prompt.provider, a.prompt.account, a.prompt.log
	go func() {
		err := officialLogin(ctx, providerID, account, func(s string) {
			log.write(s)
			if win := a.win; win != nil && !win.IsDestroyed() {
				win.Invalidate()
			}
		})
		a.apply(func() {
			a.prompt.running = false
			a.prompt.done = true
			if err != nil {
				a.prompt.err, a.prompt.errDetail = failureText("登录没有完成", err.Error())
				return
			}
			a.prompt.err = ""
			a.reloadConfig()
			a.Refresh(true)
		})
	}()
}

// --- running the sheet's action --------------------------------------------

// submitPrompt performs what the sheet asks for; the view calls it from the
// button's click handler, on the main thread.
func (a *App) submitPrompt() {
	p := &a.prompt
	switch p.kind {
	case promptAPIKey:
		key := strings.TrimSpace(p.key)
		if key == "" && p.keepKey {
			p.reset()
			a.toast = "保持原来的 API 密钥"
			return
		}
		providerID, account := p.provider, p.account
		go func() {
			err := loginAPIKey(providerID, account, key)
			a.apply(func() {
				if err != nil {
					a.prompt.err, a.prompt.errDetail = failureText("保存 API 密钥失败", err.Error())
					return
				}
				a.prompt.reset()
				a.afterCredentialChange("已保存 " + providerName(providerID) + " 的 API 密钥")
			})
		}()
	case promptCookieImport:
		providerID, account, browser, profile := p.provider, p.account, p.browser, p.profile
		go func() {
			msg, err := importCookies(providerID, account, browser, profile, true)
			a.apply(func() {
				if err != nil {
					a.prompt.err, a.prompt.errDetail = failureText("读取浏览器登录状态失败", err.Error())
					return
				}
				a.prompt.reset()
				a.afterCredentialChange(msg)
			})
		}()
	case promptManualCookie:
		providerID, account, header := p.provider, p.account, p.header
		go func() {
			err := saveManualCookie(providerID, account, header)
			a.apply(func() {
				if err != nil {
					a.prompt.err, a.prompt.errDetail = failureText("保存手动填的登录状态失败", err.Error())
					return
				}
				a.prompt.reset()
				a.afterCredentialChange("已保存手动填的登录状态")
			})
		}()
	case promptSessionFile:
		providerID, account, path := p.provider, p.account, p.path
		go func() {
			msg, err := importAuthFile(providerID, account, path)
			a.apply(func() {
				if err != nil {
					a.prompt.err, a.prompt.errDetail = failureText("导入登录文件失败", err.Error())
					return
				}
				a.prompt.reset()
				a.afterCredentialChange(msg)
			})
		}()
	case promptNewAccount:
		providerID, name := p.provider, p.name
		if err := addAccount(providerID, name); err != nil {
			p.err, p.errDetail = failureText("新建账号失败", err.Error())
			return
		}
		a.prompt.reset()
		a.reloadConfig()
		a.toast = fmt.Sprintf("已添加账号 %s（%s）", name, providerName(providerID))
	case promptCLI:
		if p.cancel != nil {
			p.cancel()
		}
		p.reset()
	}
}

// afterCredentialChange reports success, reloads the config and probes again.
func (a *App) afterCredentialChange(msg string) {
	if msg == "" {
		msg = "已更新登录信息"
	}
	a.toast = msg
	a.reloadConfig()
	a.Refresh(true)
}

// askUse asks before making an account the machine's own.
func (a *App) askUse(providerID, account string) {
	a.confirm = confirm{
		open:     true,
		title:    fmt.Sprintf("让本机的 %s 改用 %s？", providerName(providerID), displayAccount(account)),
		message:  "leeway 会把该账号的登录文件写给官方工具。官方工具当前的登录会被覆盖。",
		label:    "启用",
		kind:     "use",
		provider: providerID,
		account:  account,
	}
}

// askLogout asks before removing an account's credentials. A key-only account
// has no session to exit, so the wording says what it deletes.
func (a *App) askLogout(providerID, account string) {
	session := accountUsesSession(a, providerID, account)
	title := fmt.Sprintf("移除 %s 的账号 %s？", providerName(providerID), displayAccount(account))
	message := "会删除 leeway 里保存的这个账号的登录文件、浏览器登录状态和 API 密钥。官方工具里的登录不受影响。"
	if !session {
		title = fmt.Sprintf("删除 %s 账号 %s 的 API 密钥？", providerName(providerID), displayAccount(account))
		message = "只删 leeway 里存的这一份，别处不受影响。"
	}
	a.confirm = confirm{
		open:     true,
		title:    title,
		message:  message,
		label:    removeAccountLabel(session),
		kind:     "logout",
		provider: providerID,
		account:  account,
	}
}

// runConfirm performs the action the dialog asked about.
func (a *App) runConfirm(c confirm) {
	switch c.kind {
	case "use":
		a.Use(c.provider, c.account)
	case "logout":
		a.Logout(c.provider, c.account)
	case "clear-cache":
		a.ClearCache()
	}
}

// askClearCache asks before dropping the cached cookie imports.
func (a *App) askClearCache() {
	a.confirm = confirm{
		open:    true,
		title:   "重新读取浏览器登录状态？",
		message: "下一次查询会重新读一遍浏览器的登录状态。config.json 里的密钥和账号不受影响。",
		label:   "重新读取",
		kind:    "clear-cache",
	}
}
