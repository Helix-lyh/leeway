package cli

import (
	"fmt"
	"os"
	"strings"
)

const rootHelp = `leeway — 看各家 AI 编程额度还剩多少

  usage  [PROVIDER [ACCOUNT]]   [--json] [--watch N] [--last N]
  login  PROVIDER [ACCOUNT]     [--cookie] [--manual] [--file] [--key]
                                [--browser] [--profile] [--force]
                                [--no-browser] [--timeout N]
  logout PROVIDER [ACCOUNT]
  status                        [--json]
  use    PROVIDER ACCOUNT       [--json]
  help   [COMMAND]

每个命令做什么：
  usage   看额度和重置时间
  login   给服务商登录，或粘贴密钥
  logout  忘掉 leeway 里存的登录信息
  status  看各服务商配好了没、能不能用
  use     切换账号（写给官方工具）
  help    看帮助

PROVIDER / ACCOUNT 也可写成 --provider / --account（标签、邮箱或订阅名）。
不写 ACCOUNT 就不会去读默认账号（官方 keychain 或共享的 ~/.grok）。
看某一条的参数：leeway help usage（还有 login / logout / status / use）。
login cursor/grok/codex/kimi 用官方工具；其余粘贴 API 密钥。

退出码  0 全部正常  1 有失败或未配置  2 用法错
`

const usageHelp = `leeway usage — 看用量和余额

  leeway usage
  leeway usage cursor
  leeway usage cursor work
  leeway usage --json
  leeway usage --watch 30
  leeway usage --last 12

PROVIDER / ACCOUNT 过滤要看的服务商和账号。不写 ACCOUNT 只列出已命名账号，不读默认账号。
表上默认显示相比上次成功查询的变化（↑2 / +$12 / reset）。JSON 里始终带 delta 字段。
--last N 看近 N 次的走势，不联网。不能和 --watch 一起用。
`

const loginHelp = `leeway login — 给服务商登录

  leeway login cursor
  leeway login grok home
  leeway login opencode-go work
  leeway login commandcode goat
  leeway login deepseek --key sk-...
  leeway login opencode-go work --email you@example.com --key oc_sk-...
  leeway login grok --cookie
  leeway login grok --manual "a=b; c=d"
  leeway login grok home --file ~/.grok/auth.json

cursor / grok / codex / kimi 用官方工具登录（PATH 里要有 cursor-agent / grok / codex / kimi）。
opencode-go / commandcode / deepseek 打开申请页，粘贴 API 密钥（不回显）。
--email 记下账号归属：同一个服务商挂多个订阅时，界面和 status 靠它区分。
--email 只对粘贴密钥的服务商生效；cursor / grok / codex / kimi 的邮箱从官方登录里读。
cursor 不写 ACCOUNT 时用邮箱当账号名。
--file 把官方登录备份进 leeway 自己的目录（grok / kimi / cursor / codex），不绑正在用的官方文件。
grok 的 --file 必须是含 auth_mode / user_id 的完整 auth.json（不能用一次性登录码冒充）。每个账号网页授权一次，之后 use 只换文件。

--cookie 从浏览器读取登录状态（仅 macOS）。Linux 用 --manual。
--browser / --profile / --force 只在导入浏览器登录状态时用。
--manual 手动粘贴 Cookie。--key 直接写入 API 密钥。
--no-browser 只打印登录网址，不打开浏览器。--timeout 等浏览器授权的分钟数（默认 15）。
`

const logoutHelp = `leeway logout — 删除 leeway 存下的登录

  leeway logout cursor
  leeway logout grok home

只删 leeway 自己目录里的登录，不动官方工具和 keychain。
`

const statusHelp = `leeway status — 看各服务商登录状态

  leeway status
  leeway status --json

列出已保存的账号，不访问网络。没有账号、有账号没登录、或有服务商还没登录时退出 1。
`

const useHelp = `leeway use — 让本机官方工具改用某个账号

  leeway use cursor work
  leeway use grok home
  leeway use kimi work
  leeway use opencode-go work
  leeway use cursor work --json

cursor：写进本机所有官方 Cursor 登录位置：cursor-agent 的 keychain 项 cursor-access-token（macOS）、
  Cursor IDE 的 state.vscdb、~/.cursor/auth.json；哪个不存在就跳过，全都没有才报错（不会新建 auth.json）。
  实际写了哪几个会显示在 targets 里。Cursor 正在运行就先退出（cursor-agent 不算）。
  LEEWAY_CURSOR_KEYCHAIN=0 关掉 keychain 写入（填别的值就是 keychain 服务名）；LEEWAY_CURSOR_STATE_DB /
  LEEWAY_CURSOR_AUTH_JSON 各自把对应文件写到别处；指定任一个就不再碰真的 keychain。
grok：把完整官方登录信息合并进 ~/.grok/auth.json（信息不全的会拒绝，免得 grok 要你重新网页登录）。
kimi：有单独保存的登录就写官方 kimi-code.json（跟随 KIMI_CODE_HOME）；只有密钥的账号写进 leeway 自己保存的配置。
codex：不写官方 ~/.codex（免得刷新把官方登录弄失效）。
其余：把 API 密钥写进 leeway 自己保存的配置。

这条命令不决定 leeway 查哪个账号。要看用量：leeway usage cursor work。
只在 macOS 和 Linux 上可用。
`

var helpTopics = []struct {
	name string
	text string
}{
	{"usage", usageHelp},
	{"login", loginHelp},
	{"logout", logoutHelp},
	{"status", statusHelp},
	{"use", useHelp},
}

func helpTopicNames() string {
	names := make([]string, len(helpTopics))
	for i, t := range helpTopics {
		names[i] = t.name
	}
	return strings.Join(names, " / ")
}

func isHelpArg(s string) bool {
	return s == "-h" || s == "--help" || s == "help"
}

func printHelp(w *os.File, text string) {
	fmt.Fprint(w, strings.TrimRight(text, "\n")+"\n")
}

func helpTopic(name string) (string, bool) {
	for _, t := range helpTopics {
		if t.name == name {
			return t.text, true
		}
	}
	return "", false
}

func runHelpTopic(name string) int {
	text, ok := helpTopic(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "没有 %q 这条命令的帮助。可用：%s\n", name, helpTopicNames())
		return 2
	}
	printHelp(os.Stdout, text)
	return 0
}
