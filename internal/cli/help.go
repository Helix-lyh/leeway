package cli

import (
	"fmt"
	"os"
	"strings"
)

const rootHelp = `leeway — 查各家 AI 编程额度还剩多少

查额度
  leeway usage                                 全部已配置的订阅
  leeway usage --provider cursor               只看一家
  leeway usage --provider cursor --account work
  leeway usage --json                          给脚本 / agent
  leeway usage --watch 30                      每 30 秒刷新

管订阅
  leeway auth status                           谁已登录、何时到期
  leeway auth login --provider ID [--account TAG]
  leeway auth logout --provider ID [--account TAG]

  浏览器登录（PATH 里要有官方 CLI）
    cursor        leeway auth login --provider cursor
                  不写 --account 时用邮箱当账号名；--account work 是 tag
    grok          leeway auth login --provider grok [--account home]

  API key（会打开申请页，粘贴 key，不回显）
    opencode-go   leeway auth login --provider opencode-go --account work
    commandcode   leeway auth login --provider commandcode --account goat
    deepseek      leeway auth login --provider deepseek

  只查某一个订阅
    leeway usage --provider opencode-go --account work

  列出已存的 API key（脱敏）
    leeway config keys --provider opencode-go

  把某个订阅的 key 写给 DSH 用（不影响 leeway 自己看哪号）
    leeway use --provider opencode-go --account work

Codex / Kimi 用它们自己的登录：codex login / kimi login

体检
  leeway doctor                                缺什么、补哪条命令

其他
  leeway providers
  leeway cookie import|set|list|clear --provider ID
  leeway config path | set-api-key | keys
  leeway help COMMAND                          usage / auth / cookie / config / use / doctor / providers

--account 可以是 tag、邮箱（cursor）或订阅名（opencode-go / commandcode）。
不写时不读 default（官方 keychain / 共享 ~/.grok）；要看它请加 --account default。

退出码  0 全部正常  1 有未配置或失败  2 用法错误
`

const usageHelp = `leeway usage — 查额度

  leeway usage
  leeway usage --provider cursor
  leeway usage --provider cursor --account work
  leeway usage --json
  leeway usage --watch 30

--account 可以是 tag、邮箱或订阅名。不写只列出已命名账号，不读隐式 default（官方 keychain / 共享 ~/.grok）；要看它请加 --account default。
`

const authHelp = `leeway auth — 登录、退出、看订阅台账

  leeway auth status
  leeway auth login --provider cursor [--account work]
  leeway auth login --provider grok [--account home]
  leeway auth login --provider opencode-go --account work
  leeway auth login --provider commandcode --account goat
  leeway auth login --provider deepseek
  leeway auth logout --provider ID [--account TAG]

浏览器登录：cursor、grok（需要 cursor-agent / grok 在 PATH）。
API key：opencode-go、commandcode、deepseek（打开申请页，粘贴 key）。
logout 只删 leeway 私有凭据，不动官方 CLI / keychain。
`

const authLoginHelp = `leeway auth login — 给一家 provider 登录或写入 API key

  leeway auth login --provider cursor [--account work]
  leeway auth login --provider grok [--account home]
  leeway auth login --provider opencode-go --account work
  leeway auth login --provider commandcode --account goat
  leeway auth login --provider deepseek

cursor / grok 会拉起官方 CLI 做浏览器授权。
其余会打开 key 申请页，粘贴 key（不回显）。
`

const authLogoutHelp = `leeway auth logout — 清除 leeway 保存的登录

  leeway auth logout --provider cursor [--account work]
  leeway auth logout --provider grok [--account home]

只删 leeway 私有目录，不动官方 CLI / keychain。
API key 订阅没有私有会话可清，用 config 覆盖或改配置文件。
`

const cookieHelp = `leeway cookie — 从浏览器导入登录 cookie（grok / kimi / opencode-go 备选）

  leeway cookie import --provider grok
  leeway cookie import --provider kimi --browser kimi-desktop
  leeway cookie set --provider grok --manual "a=b; c=d"
  leeway cookie list
  leeway cookie clear --provider grok

默认 import 会读 Chrome / Edge 的 Cookie SQLite；macOS 上还要解密钥匙串
（首次会弹授权），并把对应站点的登录 cookie 写入 ~/.cache/leeway/cookies.json。
usage 不会自己刮浏览器。Linux 上这一档未实现，用 --manual 或 kimi-desktop。

grok 更推荐：leeway auth login --provider grok
`

const configHelp = `leeway config — 配置文件和 API key

  leeway config path
  leeway config set-api-key --provider ID [--account NAME] [--key VALUE]
  leeway config keys --provider ID
  leeway config set-auth-file --provider grok --account NAME --auth-file PATH

日常加订阅用 leeway auth login，效果和 set-api-key 一样。
keys 列出命名订阅（脱敏）；active 表示上次 leeway use 写进 DSH 的那把。
`

const configSetKeyHelp = `leeway config set-api-key — 写入 API key

  leeway config set-api-key --provider deepseek
  leeway config set-api-key --provider opencode-go --account work --key sk-...

不传 --key 时交互输入，不回显。日常也可以用 leeway auth login。
`

const configKeysHelp = `leeway config keys — 列出命名订阅（key 脱敏）

  leeway config keys --provider opencode-go
  leeway config keys --provider commandcode
`

const useHelp = `leeway use — 把某个订阅的 API key 写给 DSH

  leeway use --provider opencode-go --account work

这是给 DSH 热切换用的，不是「leeway 接下来看哪号」。
leeway 自己查哪号：leeway usage --provider opencode-go --account work
`

const doctorHelp = `leeway doctor — 体检：每个 provider 缺什么凭据

  leeway doctor
  leeway doctor --json
`

const providersHelp = `leeway providers — 列出 provider 和怎么登录

  leeway providers
  leeway providers --json
`

func isHelpArg(s string) bool {
	return s == "-h" || s == "--help" || s == "help"
}

func printHelp(w *os.File, text string) {
	fmt.Fprint(w, strings.TrimRight(text, "\n")+"\n")
}

func helpTopic(name string) (string, bool) {
	switch name {
	case "usage":
		return usageHelp, true
	case "auth":
		return authHelp, true
	case "login":
		return authLoginHelp, true
	case "cookie":
		return cookieHelp, true
	case "config":
		return configHelp, true
	case "use":
		return useHelp, true
	case "doctor":
		return doctorHelp, true
	case "providers":
		return providersHelp, true
	default:
		return "", false
	}
}

func runHelpTopic(name string) int {
	text, ok := helpTopic(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "没有 %q 这条命令的帮助。可用：usage / auth / cookie / config / use / doctor / providers\n", name)
		return 2
	}
	printHelp(os.Stdout, text)
	return 0
}
