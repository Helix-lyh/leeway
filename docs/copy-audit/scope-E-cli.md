# 范围 E：CLI 用户可见中文文案穷尽清点

只读分析，未修改任何代码文件。本文件是本次唯一的产物。

## 清点范围与判断口径

**读过的文件**（逐行读完）：`internal/cli/cli.go`、`help.go`、`flags.go`、`login.go`、`logout.go`、`status.go`、`auth.go`；`internal/switcher/switcher.go`、`internal/switcher/cursor_ide.go`（`switcher.go` 的 `use` 错误由 CLI 直接打印，属切号侧用户可见文案）；`internal/config/config.go`；`internal/auth/cursor_keychain.go`。

**旁证**：`internal/cli/cli_test.go`、`internal/cli/login_test.go`、`internal/switcher/switcher_test.go`（确认退出码与触发条件）；`internal/auth/login.go`、`internal/credwrite/credwrite.go`（确认这两处没有中文文案）。

**结论**：CLI 侧所有中文用户文案都落在上列文件中；`internal/config/config.go` 与 `internal/status` 相关代码**没有**中文用户文案（`status.go` 的中文只出现在英文表头数据里，见「不确定清单」）。`cmd/leeway/main.go` 只有一行中文注释，不面向用户。

**通道口径**：`flag.FlagSet` 在 `flags.go:31` 统一 `SetOutput(os.Stderr)`，因此所有 flag 用法串、`-h` 输出、用法错误都走 stderr；`leeway help` 与 `leeway help <topic>` 走 stdout（`cli.go:51`、`help.go:148`）；无参数、未知命令、未知 help 主题走 stderr。同一份帮助文案因此有两个通道（下表逐条标注，仅记录，不建议改行为——见硬约束 8）。

**改写口径**：
- 「名称 2–4 个汉字」用于概念标签（服务商 / 账号 / 登录信息）；**命令名 `usage` / `login` / `logout` / `status` / `use` / `help` 是 CLI 契约，不改**，只改它的一行说明。
- 「描述不超过 20 个汉字」用于一行说明与 flag 描述；错误提示按硬约束 5 以「原因 + 下一步」为准，超出 20 字的在备注里标 `【超20字】`。
- 术语与 `docs/UI-COPY-REWRITE.md` §1（术语对照表）对齐：provider → **服务商**，account → **账号**，会话/令牌 → **登录信息**（文件则叫**登录文件**），切号 → **切换账号**，点阵 → **走势图**，快照 → **备份**，官方 CLI/客户端 → **官方工具**，DSH → **leeway 自己保存的配置**。
- 表格里 `<br>` 表示原文的换行；多行命令示例合并成一条（给出行号范围 + 逐字原文块）。

**条数统计**：主表 137 条（help.go 50、flags.go 3、cli.go 34、login.go 13、logout.go 2、auth.go 20、switcher.go 10、cursor_ide.go 2、cursor_keychain.go 3），其中帮助文本 50 条、flag 用法描述 16 条（flags.go 3 + cli.go 4 + login.go 9）、错误与运行提示 71 条。

---

## 一、逐条对照表

| file:line | 原文案 | 命令/位置 | stdout还是stderr | 功能含义 | 建议新文案 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| internal/cli/help.go:9 | `leeway — 查各家 AI 编程额度还剩多少` | `rootHelp` 首行；`leeway`（无参数，exit 2）/ `leeway help`、`-h`、`--help`（exit 0）/ 未知子命令（exit 2） | 无参数、未知子命令 → **stderr**；`leeway help`/`-h`/`--help` → **stdout** | 一句话说明工具做什么 | `leeway — 看 AI 编程服务的用量和余额` | 「各家」是黑话；同一份文案两通道，仅记录 |
| internal/cli/help.go:11 | `  usage  [PROVIDER [ACCOUNT]]   [--json] [--watch N] [--last N]` | `rootHelp` 命令清单 | 同上 | 列 `usage` 的用法 | 保留命令名与参数（契约）；与下一行统一对齐 | 只有 usage/login 带参数行，logout/status/use 不带，句式不统一 |
| internal/cli/help.go:12-14 | `  login  PROVIDER [ACCOUNT]     [--cookie] [--manual] [--file] [--key]<br>                                [--browser] [--profile] [--force]<br>                                [--no-browser] [--timeout N]` | `rootHelp` 命令清单 | 同上 | 列 `login` 的用法 | 保留；把 `[--browser]…[--timeout N]` 收成一行 `[更多参数见 leeway help login]` | 三行占位破坏「扫一眼找到命令」；注意不能新增命令 |
| internal/cli/help.go:15 | `  logout PROVIDER [ACCOUNT]` | `rootHelp` 命令清单 | 同上 | 列 `logout` 的用法 | 保留 | 缺一行说明（同类条目只有它和 status 没有） |
| internal/cli/help.go:16 | `  status                        [--json]` | `rootHelp` 命令清单 | 同上 | 列 `status` 的用法 | 保留 | 同上 |
| internal/cli/help.go:17 | `  use    PROVIDER ACCOUNT       [--json]` | `rootHelp` 命令清单 | 同上 | 列 `use` 的用法 | 保留 | 同上 |
| internal/cli/help.go:18 | `  help   [COMMAND]` | `rootHelp` 命令清单 | 同上 | 列 `help` 的用法 | `  help   [命令]          看某条命令的说明` | 唯一没有说明的命令 |
| internal/cli/help.go:20 | `PROVIDER / ACCOUNT 也可写成 --provider / --account（tag、邮箱或订阅名）。` | `rootHelp` 参数说明 | 同上 | 说明位置参数与 flag 等价 | `服务商 / 账号 也能写成 --provider / --account（可填标签、邮箱或订阅名）。` | 「tag」是黑话；`PROVIDER/ACCOUNT` 保留占位 |
| internal/cli/help.go:21 | `不写 ACCOUNT 不读隐式 default（官方 keychain / 共享 ~/.grok）。` | `rootHelp` 参数说明 | 同上 | 说明省略账号时的行为 | `不写账号，就不会读那个没名字的默认账号（官方钥匙串 / 共用的 ~/.grok）。` | 「隐式 default」是内部概念；keychain 建议日常说法 |
| internal/cli/help.go:22 | `某一条的参数：leeway help usage（还有 login / logout / status / use）。` | `rootHelp` 帮助入口 | 同上 | 指向单命令帮助 | `看某条命令的完整参数：leeway help usage（也支持 login / logout / status / use）。` | 句式可，但「某一条」指代不清 |
| internal/cli/help.go:23 | `login cursor/grok/codex/kimi 走官方 CLI；其余粘贴 key。` | `rootHelp` 登录说明 | 同上 | 说明登录方式分两类 | `login 里 cursor / grok / codex / kimi 用官方工具登录，其它服务商粘贴 API 密钥。` | 「走」是口语但可懂；key 统一成「API 密钥」 |
| internal/cli/help.go:25 | `退出码  0 正常  1 有失败或未配置  2 用法错` | `rootHelp` 退出码说明 | 同上 | 说明退出码含义 | `退出码  0 全部正常  1 有失败或没配置  2 参数写错了` | 对齐好，保留句式 |
| internal/cli/help.go:28 | `leeway usage — 查额度` | `usageHelp` 首行；`leeway usage -h`（stderr）/ `leeway help usage`（stdout） | `-h`、用法错 → **stderr**；`help usage` → **stdout** | 一句话说明 usage | `leeway usage — 看用量和余额` | 命令名契约不改 |
| internal/cli/help.go:30-35 | `  leeway usage<br>  leeway usage cursor<br>  leeway usage cursor work<br>  leeway usage --json<br>  leeway usage --watch 30<br>  leeway usage --last 12` | `usageHelp` 示例 | 同上 | 常用示例 | 保留；建议按「最常用 → 带筛选 → 带参数」排序并加一列注释 | 示例无注释，新用户不知道 `work` 是账号 |
| internal/cli/help.go:37 | `PROVIDER / ACCOUNT 过滤要看的家和号。不写 ACCOUNT 只列出已命名账号，不读隐式 default。` | `usageHelp` 参数说明 | 同上 | 说明筛选语义 | `服务商 / 账号 用来筛选看哪家、哪个号。不写账号时只列有名字的账号，不读默认账号。` | 「家」「号」「隐式 default」三个黑话 |
| internal/cli/help.go:38 | `表上默认画相对该窗口上次成功探测的增量（↑2 / +$12 / reset）。JSON 始终带可选 delta。` | `usageHelp` 输出说明 | 同上 | 说明表格与 JSON 的差值 | `表格默认显示与上次查询的差值（↑2 / +$12 / reset）。JSON 里也带差值字段。` | 「窗口」「探测」「增量」「delta」都是术语 |
| internal/cli/help.go:39 | `--last N 看近 N 次的点阵，不联网。不能和 --watch 一起用。` | `usageHelp` 参数说明 | 同上 | 说明 `--last` | `--last N 看最近 N 次的走势图，不联网。不能和 --watch 一起用。` | 「点阵」是打印机词；与 glossary 对齐用「走势图」 |
| internal/cli/help.go:42 | `leeway login — 给一家登录或写入凭据` | `loginHelp` 首行；`leeway login -h` / `leeway login`（无 provider，exit 2）/ `leeway login 未知`（exit 2） | 全部 **stderr** | 一句话说明 login | `leeway login — 登录服务商或填 API 密钥` | 「凭据」是安全行业词，改「登录」/「密钥」 |
| internal/cli/help.go:44-52 | `  leeway login cursor<br>  leeway login grok home<br>  leeway login opencode-go work<br>  leeway login commandcode goat<br>  leeway login deepseek --key sk-...<br>  leeway login opencode-go work --email you@example.com --key oc_sk-...<br>  leeway login grok --cookie<br>  leeway login grok --manual "a=b; c=d"<br>  leeway login grok home --file ~/.grok/auth.json` | `loginHelp` 示例 | **stderr** | 常用示例 | 保留；`--key sk-...` 等示例保留占位写法（**不要**换成真实值） | 示例里的 `oc_sk-...`、`a=b; c=d` 是占位结构，安全 |
| internal/cli/help.go:54 | `cursor / grok / codex / kimi 走官方 CLI 登录（PATH 里要有 cursor-agent / grok / codex / kimi）。` | `loginHelp` 说明 | **stderr** | 说明官方登录方式 | `cursor / grok / codex / kimi 用官方工具登录（PATH 里要有 cursor-agent / grok / codex / kimi）。` | 「走官方 CLI」→「用官方工具」 |
| internal/cli/help.go:55 | `opencode-go / commandcode / deepseek 打开申请页，粘贴 API key（不回显）。` | `loginHelp` 说明 | **stderr** | 说明粘贴密钥的家 | `opencode-go / commandcode / deepseek 会打开申请页，粘贴 API 密钥（不回显）。` | 统一「API 密钥」 |
| internal/cli/help.go:56 | `--email 记下账号归属：同一家挂多个订阅时，界面和 status 靠它区分。` | `loginHelp` 参数说明 | **stderr** | 说明 `--email` 的作用 | `--email 记下账号属于谁：同一家订了多个时，用它区分窗口和 status 里的行。` | 「挂多个订阅」口语可懂；「界面」指窗口，建议统一 |
| internal/cli/help.go:57 | `--email 只对粘贴 key 的家生效；cursor / grok / codex / kimi 的邮箱从官方会话里读。` | `loginHelp` 参数说明 | **stderr** | 说明 `--email` 的适用范围 | `--email 只对粘贴密钥的服务商有效；cursor / grok / codex / kimi 的邮箱从官方登录信息里读。` | 「官方会话」→「官方登录信息」 |
| internal/cli/help.go:58 | `cursor 不写 ACCOUNT 时用邮箱当账号名。` | `loginHelp` 参数说明 | **stderr** | 说明 cursor 账号名来源 | `cursor 不写账号名时，用邮箱当账号名。` | 可，微调指代 |
| internal/cli/help.go:59 | `--file 把官方会话快照进隔离目录（grok / kimi / cursor / codex），不绑正在用的官方文件。` | `loginHelp` 参数说明 | **stderr** | 说明 `--file` | `--file 把官方登录文件备份到 leeway 自己的目录（grok / kimi / cursor / codex），不动正在用的官方文件。` | 「快照」「隔离目录」「绑」三个术语 |
| internal/cli/help.go:60 | `grok 的 --file 必须是文件里已有 auth_mode / user_id 的完整 auth.json（不要靠设备码 JWT 冒充）。每个号网页授权一次，之后 use 只换文件。` | `loginHelp` 参数说明 | **stderr** | 说明 grok `--file` 的硬要求 | `grok 的 --file 要用含 auth_mode / user_id 的完整 auth.json（只有一次性登录码的不算）。每个账号在网页授权一次，之后 use 只换文件。` | 「设备码 JWT」「冒充」都术语化；【超20字】说明类段落 |
| internal/cli/help.go:62 | `--cookie 从浏览器导入 cookie（仅 macOS）。Linux 用 --manual。` | `loginHelp` 参数说明 | **stderr** | 说明 `--cookie` | `--cookie 从浏览器导入登录状态（仅 macOS）。Linux 用 --manual。` | 与 glossary 一致：cookie → 浏览器登录状态 |
| internal/cli/help.go:63 | `--browser / --profile / --force 是导入时用的。` | `loginHelp` 参数说明 | **stderr** | 说明三个导入参数 | `--browser / --profile / --force 只在导入浏览器登录状态时用。` | 原句「是导入时用的」信息量低，未说默认值 |
| internal/cli/help.go:64 | `--manual 手动粘贴 Cookie header。--key 直接写入 API key。` | `loginHelp` 参数说明 | **stderr** | 说明 `--manual` / `--key` | `--manual 手动粘贴 Cookie 内容。--key 直接写入 API 密钥。` | 「Cookie header」术语化；`--key` 用「API 密钥」 |
| internal/cli/help.go:65 | `--no-browser 只打印登录 URL。--timeout 是等浏览器授权的分钟数（默认 15）。` | `loginHelp` 参数说明 | **stderr** | 说明 `--no-browser` / `--timeout` | `--no-browser 只打印登录网址，不打开浏览器。--timeout 等浏览器授权的分钟数（默认 15）。` | 两句可拆两行，同类参数各占一行 |
| internal/cli/help.go:68 | `leeway logout — 清掉 leeway 保存的登录` | `logoutHelp` 首行；`leeway logout -h` / `leeway logout`（无 provider，exit 2）/ `leeway logout 未知`（exit 2） | 全部 **stderr** | 一句话说明 logout | `leeway logout — 删除 leeway 存下的登录` | 与 status 的「清掉 / 移除」不统一 |
| internal/cli/help.go:70-71 | `  leeway logout cursor<br>  leeway logout grok home` | `logoutHelp` 示例 | **stderr** | 示例 | 保留 | — |
| internal/cli/help.go:73 | `只删 leeway 自己的目录，不动官方 CLI / keychain。` | `logoutHelp` 安全说明 | **stderr** | 说明删除边界 | `只删 leeway 自己目录里的登录，不动官方工具和系统钥匙串。` | 「keychain」→「系统钥匙串」 |
| internal/cli/help.go:76 | `leeway status — 各家登录了没、号还在不在` | `statusHelp` 首行；`leeway status -h`（stderr）/ `leeway help status`（stdout） | `-h`、用法错 → **stderr**；`help status` → **stdout** | 一句话说明 status | `leeway status — 看各服务商登录状态` | 「家」「号」黑话；一行说明建议 ≤20 字 |
| internal/cli/help.go:78-79 | `  leeway status<br>  leeway status --json` | `statusHelp` 示例 | 同上 | 示例 | 保留 | — |
| internal/cli/help.go:81 | `列出已保存的号，不访问网上的 provider。台账空着、有号没登录或有家缺登录时退出 1。` | `statusHelp` 行为说明 | 同上 | 说明列出内容与退出码 | `列出已保存的账号，不访问网络。没有账号、有账号没登录、或有服务商还没登录时退出 1。` | 「号」「provider」「台账」三个黑话；退出码说明好 |
| internal/cli/help.go:84 | `leeway use — 把某个号变成本机正在用的那个` | `useHelp` 首行；`leeway use -h` / `leeway use`（缺 provider/account，exit 2） | 全部 **stderr** | 一句话说明 use | `leeway use — 让本机官方工具改用某个账号` | 「某个号」「那个」指代含糊，最危险的操作说明最含糊 |
| internal/cli/help.go:86-90 | `  leeway use cursor work<br>  leeway use grok home<br>  leeway use kimi work<br>  leeway use opencode-go work<br>  leeway use cursor work --json` | `useHelp` 示例 | **stderr** | 示例 | 保留 | — |
| internal/cli/help.go:92 | `cursor：往本机所有官方 Cursor 凭据位置写：cursor-agent 的 keychain 项 cursor-access-token（macOS）、` | `useHelp` cursor 段（第 1/5 行） | **stderr** | 说明 cursor 写入位置 | `cursor：写进本机所有官方 Cursor 登录位置：cursor-agent 的钥匙串项 cursor-access-token（macOS）、` | 「凭据位置」「keychain」术语化 |
| internal/cli/help.go:93 | `  Cursor IDE 的 state.vscdb、~/.cursor/auth.json；哪一个不存在就跳过，一个都没找到才报错（不再新建 auth.json）。` | `useHelp` cursor 段（2/5） | **stderr** | 说明跳过与报错规则 | `  Cursor IDE 的 state.vscdb、~/.cursor/auth.json；哪个不存在就跳过，全都没有才报错（不会新建 auth.json）。` | 实现细节（state.vscdb）出现在第一屏 |
| internal/cli/help.go:94 | `  用的哪几个会打在 targets 里。Cursor 在跑就先退出（cursor-agent 不算）。` | `useHelp` cursor 段（3/5） | **stderr** | 说明输出与前置条件 | `  实际写了哪几个会显示在 targets 里。Cursor 正在运行就先退出（cursor-agent 不算）。` | 「打在」口语但含糊；「targets」是 JSON 字段名，需说明 |
| internal/cli/help.go:95 | `  LEEWAY_CURSOR_KEYCHAIN=0 关掉 keychain 写入（其它值是 keychain 服务名）；LEEWAY_CURSOR_STATE_DB /` | `useHelp` cursor 段（4/5） | **stderr** | 说明调试用环境变量 | `  LEEWAY_CURSOR_KEYCHAIN=0 关掉钥匙串写入（填别的值就是钥匙串服务名）；LEEWAY_CURSOR_STATE_DB /` | 开发开关出现在用户帮助第一屏 |
| internal/cli/help.go:96 | `  LEEWAY_CURSOR_AUTH_JSON 各自把对应文件写到别处，指定任一个就不再碰真 keychain。` | `useHelp` cursor 段（5/5） | **stderr** | 说明调试用环境变量 | `  LEEWAY_CURSOR_AUTH_JSON 各自把对应文件写到别处；指定任一个就不再碰真的钥匙串。` | 同上 |
| internal/cli/help.go:97 | `grok：把完整官方会话合并进 ~/.grok/auth.json（瘦设备码文件会拒绝，以免逼 grok 重新网页登录）。` | `useHelp` grok 段 | **stderr** | 说明 grok 写入行为 | `grok：把完整官方登录信息合并进 ~/.grok/auth.json（信息不全的会拒绝，免得 grok 要你重新网页登录）。` | 「会话」「瘦设备码文件」「逼」都术语/口语混杂 |
| internal/cli/help.go:98 | `kimi：有隔离会话则写官方 kimi-code.json（尊重 KIMI_CODE_HOME）；只有 key 的号写 DSH。` | `useHelp` kimi 段 | **stderr** | 说明 kimi 写入行为 | `kimi：有单独保存的登录就写官方 kimi-code.json（跟随 KIMI_CODE_HOME）；只有密钥的账号写进 leeway 自己保存的配置。` | 「隔离会话」「DSH」黑话；【超20字】段落 |
| internal/cli/help.go:99 | `codex：不写官方 ~/.codex（避免 refresh 把官方会话作废）。` | `useHelp` codex 段 | **stderr** | 说明 codex 不支持写入 | `codex：不写官方 ~/.codex（免得刷新把官方登录弄失效）。` | 「refresh」「作废」术语化 |
| internal/cli/help.go:100 | `其余：把 API key 写给 DSH。` | `useHelp` 其余服务商 | **stderr** | 说明默认写入目标 | `其余：把 API 密钥写进 leeway 自己保存的配置。` | 「DSH」黑话，与 glossary D 项对齐 |
| internal/cli/help.go:102 | `这不是「leeway 接下来查哪号」。查额度用 leeway usage cursor work。` | `useHelp` 关键澄清 | **stderr** | 澄清 use 与 usage 的区别 | `这条命令不决定 leeway 查哪个账号。要看用量：leeway usage cursor work。` | 澄清很重要，但「查哪号」黑话；建议前置到首行 |
| internal/cli/help.go:103 | `macOS / Linux。` | `useHelp` 平台说明 | **stderr** | 说明支持平台 | `只在 macOS 和 Linux 上可用。` | 两个词独占一行，缺谓语 |
| internal/cli/help.go:145 | `没有 %q 这条命令的帮助。可用：%s\n` | `runHelpTopic`；`leeway help <未知主题>`，exit 2 | **stderr** | 未知帮助主题的错误 | `没有 %q 这条命令的说明。可以看：%s` | 已给下一步（可用主题列表），是错误文案里少有的好例子 |
| internal/cli/flags.go:12 | `一家（也可写在命令后面）` | `--provider` 的用法串；`leeway usage/login/logout/use -h`（stderr）与用法错（stderr） | **stderr** | flag 说明 | `服务商（也可写在命令后面）` | 「一家」黑话，与 glossary 对齐 |
| internal/cli/flags.go:13 | `一个账号：tag、邮箱或订阅名（也可写在命令后面）` | `--account` 的用法串 | **stderr** | flag 说明 | `账号：标签、邮箱或订阅名（也可写在命令后面）` | 「tag」→「标签」 |
| internal/cli/flags.go:34 | `JSON 输出` | `--json` 的用法串（usage / status / use） | **stderr** | flag 说明 | `输出 JSON 数据` | 与其它 flag 描述句式（动词开头）不统一 |
| internal/cli/cli.go:93 | `每 N 秒刷新` | `usage --watch` 的用法串 | **stderr** | flag 说明 | `每 N 秒自动刷新一次` | 同类 flag 描述统一动词开头 |
| internal/cli/cli.go:94 | `近 N 次探测（≥1 时不联网）` | `usage --last` 的用法串 | **stderr** | flag 说明 | `看最近 N 次记录（不联网）` | 「探测」→「查询/记录」 |
| internal/cli/cli.go:95 | `不走 60s 缓存` | `usage --no-cache` 的用法串 | **stderr** | flag 说明 | `不用 60 秒内的缓存` | 「60s」中英混排 |
| internal/cli/cli.go:96 | `表上不画相对上次的增量（JSON 仍带 delta）` | `usage --no-delta` 的用法串 | **stderr** | flag 说明 | `表格不显示与上次的差值（JSON 里仍有）` | 「增量」「delta」术语 |
| internal/cli/cli.go:101 | `usage: --last 必须 ≥ 1` | `leeway usage --last 0`，exit 2 | **stderr** | 参数校验错误 | `--last 要给 1 以上的数，例如 --last 12` | 已说原因，未给可抄的例子；前缀 `usage:` 与正文中文混排 |
| internal/cli/cli.go:105 | `usage: --last 和 --watch 不能一起用` | `leeway usage --last N --watch N`，exit 2 | **stderr** | 参数互斥错误 | `--last 和 --watch 只能选一个` | 同上；说清原因但没给替代做法 |
| internal/cli/cli.go:328 | `codex 不写官方 ~/.codex（避免 refresh 把官方会话作废）。查额度：leeway usage codex ` + account | `leeway use codex <账号>`，exit 2 | **stderr** | 拒绝 codex 切号并给替代命令 | `codex 不能写进官方 ~/.codex（免得刷新让官方登录失效）。要看用量：leeway usage codex <账号>` | 「refresh」「作废」术语；命令与正文挤一行，建议换行 |
| internal/cli/cli.go:334 | `%s 没有账号 %q\n` | `leeway use <provider> <账号>` 账号不在配置里，exit 2 | **stderr** | 账号不存在 | `%s 里没有叫 %q 的账号。先看 leeway status，或添加：leeway login %s %s --key` | 缺下一步（硬约束 5）【超20字】 |
| internal/cli/cli.go:338 | `%s 账号 %q 没有 API key — leeway login %s %s --key\n` | `leeway use <provider> <账号>`，账号没有密钥，exit 2 | **stderr** | 缺密钥 | `%s 账号 %q 还没有 API 密钥。补上：leeway login %s %s --key` | 原因+下一步都有，是全文件最好的一条 |
| internal/cli/cli.go:344 | `没有配置 key 写入目标（需要文件和字段名）` | `leeway use <provider> <账号>`，`KeyTarget` 文件或字段为空，exit 2 | **stderr** | 缺写入目标 | `不知道密钥该写到哪里：请在配置里补上文件路径和字段名` | 括号里是补充而非下一步；疑似防御分支（见不确定清单） |
| internal/cli/cli.go:348 | `写入目标失败：` + err | `leeway use <provider> <账号>` 写凭据文件失败，exit 1 | **stderr** | 写入失败 | `没能写进目标文件：` + err | 只报失败，下一步需看 err 内容 |
| internal/cli/cli.go:362 | `读取 cursor 会话失败：` + err | `leeway use cursor <账号>` 读 cursor 登录失败，exit 1 | **stderr** | 读取失败 | `读不到 cursor 的登录信息：` + err | 「会话」→「登录信息」 |
| internal/cli/cli.go:366 | `cursor 账号 %q 还没登录 — leeway login cursor %s\n` | `leeway use cursor <账号>` 无令牌，exit 2 | **stderr** | 未登录 | `cursor 账号 %q 还没登录。先登录：leeway login cursor %s` | 破折号接命令，建议句号+冒号，全文件统一 |
| internal/cli/cli.go:370 | `cursor 账号 %q 的令牌已过期 — leeway login cursor %s\n` | `leeway use cursor <账号>` JWT 过期，exit 1 | **stderr** | 登录过期 | `cursor 账号 %q 的登录过期了。重新登录：leeway login cursor %s` | 「令牌」→「登录信息」 |
| internal/cli/cli.go:379 | `找不到 Cursor 的会话库：%v\n` | `leeway use cursor <账号>`，`OfficialCursorTargets()` 失败，exit 1 | **stderr** | 找不到官方存储位置 | `没找到 Cursor 的登录位置：%v` | 「会话库」是自造词；未给下一步（如确认装过 Cursor） |
| internal/cli/cli.go:384 | `保存当前 Cursor 登录失败：` + err | `leeway use cursor <账号>` 备份现有登录失败，exit 1 | **stderr** | 备份失败 | `没能备份当前的 Cursor 登录：` + err | 「保存」与「备份」应统一 |
| internal/cli/cli.go:388 | `已把当前 Cursor 登录收进 leeway 账号 %s\n` | `leeway use cursor <账号>` 成功备份，exit 0（继续） | **stderr** | 提示已备份 | `已把当前 Cursor 登录存成 leeway 账号 %s` | 「收进」口语但方向清楚 |
| internal/cli/cli.go:390 | `当前 Cursor 登录不在 leeway 台账里，切走之后只剩目标号` | `leeway use cursor <账号>` 现有登录无法备份，exit 0（继续） | **stderr** | 警告会丢失当前登录 | `当前 Cursor 登录没存进 leeway，切走后这个号就找不回来了` | 「台账」「切走」「目标号」三个黑话；这是数据丢失警告，应最直白 |
| internal/cli/cli.go:393 | `写入 Cursor 会话失败：` + err | `leeway use cursor <账号>`，`ApplyCursor` 失败，exit 1 | **stderr** | 写入失败 | `没能写入 Cursor 的登录信息：` + err | 「会话」→「登录信息」 |
| internal/cli/cli.go:397 | `重新打开 Cursor 后才会用新号` | `leeway use cursor <账号>` 成功且非 JSON，exit 0 | **stderr** | 生效时机提示 | `要重新打开 Cursor 才会用新账号` | 「新号」→「新账号」；提示很好，建议保留位置 |
| internal/cli/cli.go:411 | `grok 账号 %q 还没登录 — leeway login grok %s\n` | `leeway use grok <账号>` 没有登录文件，exit 2 | **stderr** | 未登录 | `grok 账号 %q 还没登录。先登录：leeway login grok %s` | 与 cursor 同类，句式需统一 |
| internal/cli/cli.go:415 | `grok 账号 %q 的会话文件不在 %s — leeway login grok %s\n` | `leeway use grok <账号>` 文件不存在，exit 2 | **stderr** | 登录文件缺失 | `grok 账号 %q 的登录文件不在 %s。重新登录：leeway login grok %s` | 「会话文件」→「登录文件」 |
| internal/cli/cli.go:419 | `grok 账号 %q 还不是官方完整会话，写入会逼 grok 重新网页登录。先用能开 grok 的文件快照：leeway login grok %s --file ~/.grok/auth.json\n` | `leeway use grok <账号>` 文件缺官方字段，exit 1 | **stderr** | 拒绝写入不完整登录 | `grok 账号 %q 不是官方完整登录，写进去会让 grok 要求重新网页登录。改用能打开 grok 的文件备份：leeway login grok %s --file ~/.grok/auth.json` | 原因+下一步都有；「逼」「快照」术语【超20字】 |
| internal/cli/cli.go:424 | `找不到 grok 的会话文件：` + err | `leeway use grok <账号>`，`OfficialGrokAuthFile()` 失败，exit 1 | **stderr** | 找不到官方路径 | `找不到 grok 的登录文件：` + err | 与 cli.go:379 同类，句式统一 |
| internal/cli/cli.go:428 | `写入 grok 会话失败：` + err | `leeway use grok <账号>`，`ApplyGrok` 失败，exit 1 | **stderr** | 写入失败 | `没能写入 grok 的登录信息：` + err | 「会话」→「登录信息」 |
| internal/cli/cli.go:593 | `%s 用 --key，不支持 --file\n` | `leeway login <非 grok/kimi/cursor/codex> --file <路径>`，exit 2 | **stderr** | `--file` 用错家 | `%s 只能用 --key 填密钥，不支持 --file` | 缺下一步；说清了哪个 flag 可用 |
| internal/cli/cli.go:630 | `写入 grok 会话失败：` + err | `leeway login grok <账号> --file <路径>` 写临时文件失败，exit 1 | **stderr** | 写入失败 | `没能写入 grok 的登录信息：` + err | 与 cli.go:428 同一句话，重复 |
| internal/cli/cli.go:635 | `这份文件缺官方资料字段（auth_mode / user_id）。请先用官方 grok 登录到能正常开 CLI，再 --file 那份 ~/.grok/auth.json` | `leeway login grok <账号> --file <路径>` 校验不通过，exit 1 | **stderr** | 文件不合格 | `这份文件少了官方字段（auth_mode / user_id）。先用官方 grok 登录到能正常打开，再 --file 那份 ~/.grok/auth.json` | 原因+下一步都有；「开 CLI」略生硬 |
| internal/cli/cli.go:640 | `写入 grok 会话失败：` + err | 同上，`os.Rename` 失败，exit 1 | **stderr** | 写入失败 | `没能写入 grok 的登录信息：` + err | 第三次重复同一句 |
| internal/cli/cli.go:688 | `这份文件不是可用的 %s 会话\n` | `leeway login kimi/cursor/codex <账号> --file <路径>` 校验不通过，exit 1 | **stderr** | 文件不合格 | `这份文件不能当 %s 的登录用：缺少 access token 或 refresh token。请换一份官方登录文件` | 缺下一步（硬约束 5）【超20字】 |
| internal/cli/cli.go:724 | `已快照 %s 账号 %s → %s\n` | `leeway login grok/kimi/cursor/codex <账号> --file` 成功，exit 0 | **stderr** | 备份成功 | `已备份 %s 账号 %s → %s` | 「快照」→「备份」 |
| internal/cli/cli.go:779 | `kimi 账号 %q 的会话文件不在 %s — leeway login kimi %s\n` | `leeway use kimi <账号>` 文件不存在，exit 2 | **stderr** | 登录文件缺失 | `kimi 账号 %q 的登录文件不在 %s。重新登录：leeway login kimi %s` | 与 cli.go:415 句式对齐 |
| internal/cli/cli.go:793 | `kimi 账号 %q 不是完整 CLI 会话 — leeway login kimi %s\n` | `leeway use kimi <账号>` 缺 token，exit 1 | **stderr** | 登录不完整 | `kimi 账号 %q 不是完整登录。重新登录：leeway login kimi %s` | 「CLI 会话」术语 |
| internal/cli/cli.go:797 | `kimi 账号 %q 绑在官方会话文件上。先快照：leeway login kimi %s --file %s\n` | `leeway use kimi <账号>` 源就是官方文件，exit 1 | **stderr** | 拒绝自覆盖 | `kimi 账号 %q 直接绑在官方登录文件上。先备份：leeway login kimi %s --file %s` | 「绑」「快照」术语；原因清楚 |
| internal/cli/login.go:37 | `从浏览器导入 cookie` | `login --cookie` 的用法串 | **stderr** | flag 说明 | `从浏览器导入登录状态` | 与 glossary 对齐 |
| internal/cli/login.go:38 | `Cookie header，例如 a=b; c=d` | `login --manual` 的用法串 | **stderr** | flag 说明 | `手动粘贴的 Cookie 内容，例如 a=b; c=d` | 「Cookie header」术语；示例是占位结构 |
| internal/cli/login.go:39 | `会话文件路径` | `login --file` 的用法串 | **stderr** | flag 说明 | `官方登录文件的路径` | 「会话文件」→「登录文件」 |
| internal/cli/login.go:40 | `API key（省略则交互输入，不回显）` | `login --key` 的用法串 | **stderr** | flag 说明 | `API 密钥（不写就交互输入，不回显）` | 统一「API 密钥」 |
| internal/cli/login.go:41 | `账号邮箱，用来区分同一 provider 的多个账号` | `login --email` 的用法串 | **stderr** | flag 说明 | `账号邮箱，用来区分同一服务商的多个账号` | 「provider」→「服务商」 |
| internal/cli/login.go:42 | `chrome \| edge \| kimi-desktop（默认 chrome）` | `login --browser` 的用法串 | **stderr** | flag 说明 | 保留取值列表；改为 `用哪个浏览器：chrome / edge / kimi-desktop（默认 chrome）` | 原文含 `\|`；缺「这是什么」的开头 |
| internal/cli/login.go:44 | `忽略缓存，强制重导` | `login --force` 的用法串 | **stderr** | flag 说明 | `忽略已有缓存，重新导入一次` | 「重导」是缩写 |
| internal/cli/login.go:45 | `只打印登录 URL，不打开浏览器` | `login --no-browser` 的用法串 | **stderr** | flag 说明 | `只打印登录网址，不打开浏览器` | 「URL」中英混排 |
| internal/cli/login.go:46 | `等待浏览器授权的分钟数` | `login --timeout` 的用法串 | **stderr** | flag 说明 | `等浏览器授权的分钟数（默认 15）` | 帮助正文写了默认 15，flag 串里没写 |
| internal/cli/login.go:183 | `没找到 codex CLI，先安装官方 Codex CLI，再重试` | `leeway login codex` 找不到 `codex`，exit 2 | **stderr** | 缺官方命令 | `没找到 codex 命令：先装官方 Codex CLI，再重试` | 原因+下一步都有；可给安装入口 |
| internal/cli/login.go:220 | `\n已登录 codex 账号 %s → %s\n` | `leeway login codex <账号>` 成功，exit 0 | **stderr** | 登录成功 | `\n已登录 codex 账号 %s → %s\n` | 成功提示，保留；与 kimi/grok 句式一致 |
| internal/cli/login.go:229 | `没找到 kimi CLI，改为粘贴 API key` | `leeway login kimi` 找不到 `kimi`，自动降级 | **stderr** | 降级提示 | `没找到 kimi 命令，改成粘贴 API 密钥` | 「改为」是陈述而非提示下一步，可接受 |
| internal/cli/login.go:265 | `\n已登录 kimi 账号 %s → %s\n` | `leeway login kimi <账号>` 成功，exit 0 | **stderr** | 登录成功 | 保留 | 与 codex 行完全同构，好 |
| internal/cli/logout.go:72 | `已清除 %s 账号 %s 的私有登录（%s）\n` | `leeway logout codex/kimi <账号>` 成功，exit 0 | **stderr** | 清除成功 | `已删除 %s 账号 %s 的登录（%s）` | 「私有登录」是自造词；与 auth.go:548/578 三种说法不统一 |
| internal/cli/logout.go:99 | `已移除 %s 账号 %s\n` | `leeway logout <密钥类家> <账号>` 成功，exit 0 | **stderr** | 移除成功 | `已删除 %s 账号 %s` | 「清除 / 移除 / 删除」三种动词混用 |
| internal/cli/auth.go:39 | `没找到 cursor-agent CLI，先安装（https://cursor.com/cli），再重试` | `leeway login cursor` 找不到 `cursor-agent`，exit 2 | **stderr** | 缺官方命令 | `没找到 cursor-agent 命令：先按 https://cursor.com/cli 安装，再重试` | 原因+下一步+链接都有，是好例子 |
| internal/cli/auth.go:79 | `登录似乎没完成：%s 里没有令牌\n` | `leeway login cursor` 官方 CLI 跑完但没写出令牌，exit 1 | **stderr** | 登录未完成 | `登录好像没完成：%s 里没有登录信息。请重跑 leeway login cursor` | 缺下一步（硬约束 5） |
| internal/cli/auth.go:114 | `，有效期到 ` | 拼进 `auth.go:118` 的片段 | **stderr** | 追加有效期 | `，有效期到 ` | 片段，单独看无意义，见不确定清单 |
| internal/cli/auth.go:118 | `\n已登录 cursor 账号 %s%s，令牌由官方 CLI 写入 %s（0600）\n` | `leeway login cursor` 成功，exit 0 | **stderr** | 登录成功 | `\n已登录 cursor 账号 %s%s，登录信息由官方工具写入 %s（权限 0600）\n` | 「令牌」「0600」对用户是噪音，可保留文件权限说明但加一句人话 |
| internal/cli/auth.go:119 | `该账号与官方 Cursor CLI / keychain 完全独立。查额度在 usage 后面加号；要让本机 Cursor 用这个号：` | `leeway login cursor` 成功提示第 2 行 | **stderr** | 说明隔离性与后续命令 | `这个账号和官方 Cursor 工具、系统钥匙串互不影响。要看用量，在 usage 后面加账号名；要让本机 Cursor 用这个账号：` | 「号」→「账号」；「keychain」→「系统钥匙串」；句子长【超20字】 |
| internal/cli/auth.go:141 | `grok 账号 %s 已有完整官方快照。切本机：leeway use grok %s；替换快照：leeway login grok %s --file ~/.grok/auth.json\n` | `leeway login grok <账号>` 已有完整登录，exit 1 | **stderr** | 拒绝重复登录并给两条出路 | `grok 账号 %s 已有一份完整官方登录。让本机 grok 用它：leeway use grok %s；换一份：leeway login grok %s --file ~/.grok/auth.json` | 「快照」「切本机」术语；两个下一步都给了，好 |
| internal/cli/auth.go:147 | `没找到 grok CLI，先安装：curl -fsSL https://x.ai/cli/install.sh \| bash` | `leeway login grok` 找不到 `grok`，exit 2 | **stderr** | 缺官方命令 | `没找到 grok 命令：先安装 curl -fsSL https://x.ai/cli/install.sh \| bash` | 原因+下一步都有；原文含 `\|` |
| internal/cli/auth.go:174 | `\n已登录 grok 账号 %s → %s\n` | `leeway login grok <账号>` 成功，exit 0 | **stderr** | 登录成功 | 保留 | 与 codex/kimi 同构 |
| internal/cli/auth.go:175 | `该账号与官方 grok CLI（~/.grok）完全独立。查额度：leeway usage grok` + suffixFlag(name) | `leeway login grok` 成功提示第 2 行 | **stderr** | 说明隔离性与查用量命令 | `这个账号和官方 grok 工具（~/.grok）互不影响。看用量：leeway usage grok <账号>` | 「查额度」与 help 里的「查额度/用量」不统一 |
| internal/cli/auth.go:177 | `要让本机 grok CLI 用这个号：` | `leeway login grok` 成功提示（文件完整时） | **stderr** | 引出下一步命令 | `要让本机 grok 用这个账号：` | 「号」→「账号」 |
| internal/cli/auth.go:180 | `设备码会话缺官方资料字段，use 会拒绝写入。切本机 grok 请先快照能开 CLI 的文件：leeway login grok %s --file ~/.grok/auth.json\n` | `leeway login grok` 成功但文件不完整 | **stderr** | 警告 use 会拒绝 | `这份登录缺官方字段，use 会拒绝写入。要让本机 grok 用它，先备份能打开 grok 的文件：leeway login grok %s --file ~/.grok/auth.json` | 「设备码会话」「快照」「切本机」术语堆叠【超20字】 |
| internal/cli/auth.go:202 | `打开 %s 创建并复制 API key…\n` | `leeway login deepseek/opencode-go/commandcode` 无 `--key` 且会开浏览器 | **stderr** | 提示打开申请页 | `正在打开 %s，请创建并复制 API 密钥…` | 「API key」→「API 密钥」 |
| internal/cli/auth.go:204 | `(打不开浏览器，请手动访问 %s)\n` | 上一步 `auth.OpenBrowser` 失败 | **stderr** | 打开失败兜底 | `(没能打开浏览器，请手动访问 %s)` | 原因+下一步都有，好 |
| internal/cli/auth.go:208 | `粘贴 API key（不回显）: ` | `auth.PromptSecret` 的交互提示，`leeway login <密钥类家>` 无 `--key` | **stderr**（提示符随终端） | 交互输入提示 | `粘贴 API 密钥（不回显）: ` | 「API key」→「API 密钥」；末尾用半角冒号，建议全角或统一 |
| internal/cli/auth.go:237 | `保存邮箱失败：` + err | `leeway login <家> --email <邮箱>` 写配置失败，exit 1 | **stderr** | 保存失败 | `没能保存邮箱：` + err | 与其它「…失败」前缀句式统一即可 |
| internal/cli/auth.go:242 | `已保存 %s 的 key（%s）\n` | `leeway login <密钥类家>` 成功，exit 0 | **stderr** | 保存成功 | `已保存 %s 的 API 密钥（%s）` | 「key」→「API 密钥」；`maskKey` 只显示前 6 位，不泄露密钥 |
| internal/cli/auth.go:244 | `suffixAccount("查额度：leeway usage "` + providerID, account) | `leeway login <家> <账号>` 成功提示（有账号时） | **stderr** | 给出查用量命令 | `看用量：leeway usage <provider> <account>` | 「查额度」与「查用量」两种叫法 |
| internal/cli/auth.go:246 | `查额度：leeway usage ` + providerID | `leeway login <家>` 成功提示（无账号时） | **stderr** | 给出查用量命令 | `看用量：leeway usage <provider>` | 同上；与 auth.go:175 拼接方式不同，见不确定清单 |
| internal/cli/auth.go:548 | `已清除 cursor 账号的私有登录（%s）；keychain 里的官方会话不受影响\n` | `leeway logout cursor <账号>` 成功，exit 0 | **stderr** | 清除成功并说明边界 | `已删除 cursor 账号的登录（%s）；系统钥匙串里的官方登录不受影响` | 「私有登录」「keychain」「官方会话」三个术语 |
| internal/cli/auth.go:578 | `已清除 grok 账号 %q 的私有登录（%s）\n` | `leeway logout grok <账号>` 成功，exit 0 | **stderr** | 清除成功 | `已删除 grok 账号 %q 的登录（%s）` | 与 logout.go:72、auth.go:548 三处句式不同 |
| internal/switcher/cursor_ide.go:17 | `找不到 pgrep，无法确认 Cursor 是否在跑。先退出 Cursor，或把 pgrep 装上` | `leeway use cursor <账号>`，`CheckCursorIDE()` 前置检查，经 `cli.go:374` 原样打到 stderr，exit 1 | **stderr** | 环境缺工具 | `找不到 pgrep，没法确认 Cursor 是否在运行。先退出 Cursor，或装上 pgrep` | 原因+两个下一步都有，是全项目最好的错误之一；「在跑」口语 |
| internal/switcher/cursor_ide.go:20 | `Cursor 正在运行。先退出再切号，避免写坏 state.vscdb` | 同上，Cursor 在运行 | **stderr** | 阻止写入 | `Cursor 正在运行。先退出再切换账号，免得写坏 state.vscdb` | 「切号」→「切换账号」；原因+下一步都有 |
| internal/switcher/switcher.go:26 | `给本机官方客户端切号只支持 macOS 和 Linux` | `errUnsupportedOS`；`OfficialCursorTargets()` / `OfficialGrokAuthFile()` 在非 macOS/Linux 返回，经 `cli.go:379` / `cli.go:424` 包装后打印，exit 1 | **stderr** | 平台不支持 | `只有 macOS 和 Linux 能把登录切换给官方工具` | 「切号」术语；被包装后语义错位（见不确定清单） |
| internal/switcher/switcher.go:145 | `找到 %d 个 Cursor state.vscdb（%s）——用 LEEWAY_CURSOR_STATE_DB 指定要写哪一个` | `pickCursorStateDB`；`leeway use cursor <账号>` 找到多个 db，经 `cli.go:379` 包装，exit 1 | **stderr** | 多个候选需指定 | `找到 %d 个 Cursor state.vscdb（%s）；请用 LEEWAY_CURSOR_STATE_DB 指定写哪一个` | 原因+下一步都有；`——` 与全项目 `—` 不统一 |
| internal/switcher/switcher.go:168 | `cursor 会话里没有 access token` | `ApplyCursor`；`leeway use cursor <账号>`，经 `cli.go:393` 包装，exit 1 | **stderr** | 登录信息不全 | `cursor 的登录信息里没有 access token` | 「会话」→「登录信息」 |
| internal/switcher/switcher.go:171 | `这台机器上没找到可写的 Cursor 凭据位置（IDE 的 state.vscdb、cursor-agent 的 keychain、~/.cursor/auth.json 都没有）` | `ApplyCursor`；经 `cli.go:393` 包装，exit 1 | **stderr** | 无可写位置 | `这台机器上没找到能写 Cursor 登录的地方（IDE 的 state.vscdb、cursor-agent 的钥匙串、~/.cursor/auth.json 都没有）。请先装并登录 Cursor` | 「凭据位置」「keychain」术语；建议补下一步【超20字】 |
| internal/switcher/switcher.go:197 | `cursor state.vscdb 的路径是空的` | `writeCursorStateDB`；防御分支，经 `cli.go:393` 包装，exit 1 | **stderr** | 内部参数为空 | `cursor state.vscdb 的路径是空的` | 用户无法采取行动，属内部错误；见不确定清单 |
| internal/switcher/switcher.go:208 | `打开 Cursor state.vscdb：%w` | `writeCursorStateDB` 打不开 sqlite，经 `cli.go:393` 包装，exit 1 | **stderr** | 打开失败 | `打不开 Cursor 的 state.vscdb：%w` | 可；「打开」略歧义（读还是写） |
| internal/switcher/switcher.go:244 | `写入 Cursor state.vscdb：%w` | `writeCursorStateDB` 提交事务失败，经 `cli.go:393` 包装，exit 1 | **stderr** | 写入失败 | `没能写入 Cursor 的 state.vscdb：%w` | 与 `switcher.go:208` 句式不齐 |
| internal/switcher/switcher.go:306 | `读本机 cursor keychain 失败，已放弃切换（否则会丢当前登录）：%v` | `SnapshotCursorIfNeeded`；经 `cli.go:384` 包装，exit 1 | **stderr** | 读不到当前登录，放弃切换 | `读不到本机 cursor 的钥匙串登录，已放弃切换（不然会丢掉当前登录）：%v` | 原因+后果清楚，好；「keychain」→「钥匙串」 |
| internal/switcher/switcher.go:428 | `grok 会话里没有 xAI 登录信息` | `mergeGrokAuth`；经 `cli.go:428` 包装，exit 1 | **stderr** | 源文件缺 xAI 条目 | `grok 的登录文件里没有 xAI 登录信息` | 「会话」→「登录文件」 |
| internal/switcher/switcher.go:447 | `grok auth.json 不是 JSON 对象` | `grokRegistry`；经 `cli.go:428` 包装，exit 1 | **stderr** | 文件格式不对 | `grok 的 auth.json 不是 JSON 对象，请换一份官方登录文件` | 缺下一步 |
| internal/auth/cursor_keychain.go:69 | `写 Cursor keychain 失败：%v` | `SetCursorKeychainToken`；经 `cli.go:393` 包装，exit 1 | **stderr** | 钥匙串写入失败 | `没能写进 Cursor 的钥匙串：%v` | 「keychain」→「钥匙串」；macOS 可能弹权限框，建议提示 |
| internal/auth/cursor_keychain.go:73 | `写 Cursor keychain 后回读失败：%v` | 同上，写入后回读失败 | **stderr** | 校验失败 | `写进 Cursor 钥匙串后读不回来：%v` | 用户不知怎么办，建议补「请重试或检查钥匙串权限」【超20字】 |
| internal/auth/cursor_keychain.go:76 | `写 Cursor keychain 后回读不一致：写入 %d 字节，读回 %d 字节` | 同上，写入与读回长度不一致 | **stderr** | 校验失败 | `写进 Cursor 钥匙串的内容对不上：写入 %d 字节，读回 %d 字节。请重试` | 只报事实；「字节」对用户是噪音但说明了原因 |

---

## 二、命令一览表

现在 `rootHelp` 里 `usage / login / logout / status / use` 都**没有**一行说明（只有 `help` 之后的散文段）。下面是每个子命令的「现在是什么 / 建议改成什么」，一行说明控制在 20 个汉字内，句式为「动词 + 对象」。

| 命令 | 现在的一行说明 | 出处 | 建议的一行说明 | 建议后字数 |
| --- | --- | --- | --- | --- |
| `usage` | 查额度（仅 `usageHelp` 首行有，`rootHelp` 里没有） | help.go:11、help.go:28 | 看用量和余额 | 6 |
| `login` | 给一家登录或写入凭据（仅 `loginHelp` 首行有） | help.go:12-14、help.go:42 | 登录服务商或填密钥 | 8 |
| `logout` | 清掉 leeway 保存的登录（仅 `logoutHelp` 首行有） | help.go:15、help.go:68 | 删除已保存的登录 | 7 |
| `status` | 各家登录了没、号还在不在（仅 `statusHelp` 首行有） | help.go:16、help.go:76 | 看各服务商登录状态 | 9 |
| `use` | 把某个号变成本机正在用的那个（仅 `useHelp` 首行有） | help.go:17、help.go:84 | 让官方工具改用某账号 | 9 |
| `help` | 无 | help.go:18 | 看某条命令的说明 | 8 |

**建议的 `rootHelp` 命令清单形态**（同类条目同一句式：命令 + 参数 + 一行说明，说明左对齐成列）：

```
  usage  [服务商 [账号]]   看用量和余额      [--json] [--watch N] [--last N]
  login  服务商 [账号]     登录服务商或填密钥
  logout 服务商 [账号]     删除已保存的登录
  status                  看各服务商登录状态  [--json]
  use    服务商 账号       让官方工具改用某账号 [--json]
  help   [命令]           看某条命令的说明
```

命令名与参数是 CLI 契约（AGENTS.md：`cmd/leeway` 的 CLI 行为是契约），本表只改说明文字，不动命令名、参数与输出通道。

---

## 三、错误与下一步

判断标准：每条错误是否同时说清**原因**（为什么会这样）和**下一步**（我现在该敲什么）。「下一步」必须是一条可照抄的命令或一个明确动作。

| file:line | 原文案（截断到关键部分） | 说清原因？ | 说清下一步？ | 建议补什么 |
| --- | --- | --- | --- | --- |
| cli.go:101 | `usage: --last 必须 ≥ 1` | 是 | **否** | 给可照抄的例子：`例如 --last 12` |
| cli.go:105 | `usage: --last 和 --watch 不能一起用` | 是 | **否** | 说明二者只能选一个，并给推荐写法 |
| cli.go:328 | `codex 不写官方 ~/.codex…查额度：leeway usage codex …` | 是 | 是 | 仅术语问题（refresh / 作废） |
| cli.go:334 | `%s 没有账号 %q` | 是 | **否** | 加 `先看 leeway status，或 leeway login %s %s --key` |
| cli.go:338 | `%s 账号 %q 没有 API key — leeway login …` | 是 | 是 | 只改术语（API key → API 密钥） |
| cli.go:344 | `没有配置 key 写入目标（需要文件和字段名）` | 是 | **否** | 指向配置文件要补的字段；触发路径存疑（见不确定清单） |
| cli.go:348 | `写入目标失败：` + err | 部分 | **否** | 说明失败后数据是否已改（本次未写入），并提示重试 |
| cli.go:362 | `读取 cursor 会话失败：` + err | 部分 | **否** | 建议加 `请重跑 leeway login cursor` |
| cli.go:366 | `cursor 账号 %q 还没登录 — leeway login cursor %s` | 是 | 是 | 破折号改「先登录：」 |
| cli.go:370 | `cursor 账号 %q 的令牌已过期 — leeway login cursor %s` | 是 | 是 | 「令牌」→「登录」 |
| cli.go:379 | `找不到 Cursor 的会话库：%v` | 是 | **否** | 加「确认装过 Cursor，或用 LEEWAY_CURSOR_STATE_DB 指定路径」 |
| cli.go:384 | `保存当前 Cursor 登录失败：` + err | 部分 | **否** | 说明此时**没有**切换，可重试 |
| cli.go:390 | `当前 Cursor 登录不在 leeway 台账里，切走之后只剩目标号` | 是 | 部分 | 说明后果（当前号找不回）并给「先 login 存一份」的办法 |
| cli.go:393 | `写入 Cursor 会话失败：` + err | 部分 | **否** | 说明可能只写了一部分（钥匙串优先写），建议重试 |
| cli.go:411 | `grok 账号 %q 还没登录 — leeway login grok %s` | 是 | 是 | 与 cursor 行统一句式 |
| cli.go:415 | `grok 账号 %q 的会话文件不在 %s — leeway login grok %s` | 是 | 是 | 「会话文件」→「登录文件」 |
| cli.go:419 | `grok 账号 %q 还不是官方完整会话…先用能开 grok 的文件快照：…` | 是 | 是 | 术语（逼 / 快照） |
| cli.go:424 | `找不到 grok 的会话文件：` + err | 是 | **否** | 与 cli.go:379 同样补「确认官方工具装过」 |
| cli.go:428 / 630 / 640 | `写入 grok 会话失败：` + err | 部分 | **否** | 三处同一句，建议统一为一条并说明可重试 |
| cli.go:593 | `%s 用 --key，不支持 --file` | 是 | 部分 | 给一条示例命令 |
| cli.go:635 | `这份文件缺官方资料字段（auth_mode / user_id）。请先用官方 grok 登录…` | 是 | 是 | 术语（开 CLI） |
| cli.go:688 | `这份文件不是可用的 %s 会话` | 是（笼统） | **否** | 说清缺什么（access/refresh token）+「换一份官方登录文件」 |
| cli.go:779 | `kimi 账号 %q 的会话文件不在 %s — leeway login kimi %s` | 是 | 是 | 与 grok/cursor 行统一 |
| cli.go:793 | `kimi 账号 %q 不是完整 CLI 会话 — leeway login kimi %s` | 是 | 是 | 「CLI 会话」→「登录」 |
| cli.go:797 | `kimi 账号 %q 绑在官方会话文件上。先快照：…` | 是 | 是 | 术语（绑 / 快照） |
| login.go:183 | `没找到 codex CLI，先安装官方 Codex CLI，再重试` | 是 | 是 | 可补安装入口（cursor 那条有链接） |
| login.go:229 | `没找到 kimi CLI，改为粘贴 API key` | 是 | 是（隐含） | 明确「现在开始粘贴密钥」 |
| logout.go:57 | `refuse to delete %s (not a leeway session dir)`（**英文**） | 是 | **否** | 与中文兄弟条目 `已清除…` 不成对，建议中文化并说明已中止 |
| auth.go:79 | `登录似乎没完成：%s 里没有令牌` | 是 | **否** | 加「请重跑 leeway login cursor」 |
| auth.go:141 | `grok 账号 %s 已有完整官方快照。切本机：…；替换快照：…` | 是 | 是 | 两个下一步都给了，只改术语 |
| auth.go:147 | `没找到 grok CLI，先安装：curl …` | 是 | 是 | 好例子 |
| auth.go:180 | `设备码会话缺官方资料字段，use 会拒绝写入。切本机 grok 请先快照…` | 是 | 是 | 术语堆叠，可读性差 |
| auth.go:204 | `(打不开浏览器，请手动访问 %s)` | 是 | 是 | 好例子 |
| auth.go:237 | `保存邮箱失败：` + err | 部分 | **否** | 说明账号已创建但邮箱没记上 |
| cursor_ide.go:17 | `找不到 pgrep，无法确认 Cursor 是否在跑。先退出 Cursor，或把 pgrep 装上` | 是 | 是 | 全项目最好的错误之一，作为句式样板 |
| cursor_ide.go:20 | `Cursor 正在运行。先退出再切号，避免写坏 state.vscdb` | 是 | 是 | 只改「切号」 |
| switcher.go:145 | `找到 %d 个 Cursor state.vscdb（%s）——用 LEEWAY_CURSOR_STATE_DB 指定…` | 是 | 是 | 破折号统一 |
| switcher.go:171 | `这台机器上没找到可写的 Cursor 凭据位置（…）` | 是 | **否** | 加「请先装并登录 Cursor」 |
| switcher.go:197 | `cursor state.vscdb 的路径是空的` | 是 | **否** | 用户无法行动，属内部错误 |
| switcher.go:306 | `读本机 cursor keychain 失败，已放弃切换（否则会丢当前登录）：%v` | 是 | 部分 | 加「请重试」；术语 keychain |
| switcher.go:428 | `grok 会话里没有 xAI 登录信息` | 是 | **否** | 指向 `leeway login grok --file` |
| switcher.go:447 | `grok auth.json 不是 JSON 对象` | 是 | **否** | 加「换一份官方登录文件」 |
| cursor_keychain.go:69 | `写 Cursor keychain 失败：%v` | 部分 | **否** | 提示钥匙串权限弹窗 / 重试 |
| cursor_keychain.go:73 | `写 Cursor keychain 后回读失败：%v` | 是 | **否** | 加「请重试或检查钥匙串权限」 |
| cursor_keychain.go:76 | `写 Cursor keychain 后回读不一致：写入 %d 字节，读回 %d 字节` | 是 | **否** | 加「请重试」 |

**统计**：本节 45 条错误/警告里，原因说清的 38 条（37 条「是」+ 1 条「是（笼统）」），说不全的 7 条；**下一步明确给全的只有 19 条，另有 1 条隐含、3 条只给一半，完全没给的 22 条**——也就是说近一半错误只说「错了」不说「怎么办」。缺下一步最集中的是 `use`（cli.go:334/344/379/384/390/393）与 `switcher`（171/428/447）。

---

## 四、不确定清单

1. **`--last N 看近 N 次的点阵`**：实际渲染由 `internal/render` 的 `SparkBlock` 决定，我没在本次范围内读它的画法，不确定「走势图」是否比「点阵」更贴合真实形态。建议改名，但形态确认后再定词。
2. **`没有配置 key 写入目标（需要文件和字段名）`（cli.go:344）是否可达**：`config.KeyTarget` 总会返回 DSH 默认文件与字段名，只有配置里显式写成空串才可能触发。我判断它接近防御分支，但不确定是否有其它调用路径；若不可达，建议连同文案一起确认。
3. **`switcher.go:197 cursor state.vscdb 的路径是空的`**：同上是内部防御分支，用户看到也无法行动。是否保留、是否改成「内部错误，请反馈」需要作者定。
4. **`switcher.go:26` 的包装错位**：`errUnsupportedOS` 是「给本机官方客户端切号只支持 macOS 和 Linux」，但它会被 `cli.go:424` 包装成「找不到 grok 的会话文件：给本机官方客户端切号只支持 macOS 和 Linux」。我只记录这个语义错位，**不建议改包装行为**，改哪一层由作者定。
5. **`auth.go:114` 的 `，有效期到 ` 是拼接片段**：整句是「已登录 cursor 账号 X，有效期到 Y，令牌由官方 CLI 写入 Z（0600）」。单独改这个片段会牵动整句语序，是否拆成两行需要产品决定。
6. **`useHelp` 第一屏的 `LEEWAY_CURSOR_*` 环境变量与 `state.vscdb`**：这些更像开发文档。我建议移出用户帮助或折叠到末尾，但不确定作者是否有意让它们出现在 `leeway help use`。
7. **`leeway help <命令>` 走 stdout、`leeway <命令> -h` 走 stderr，同一份文案两个通道**：按硬约束 8 只做记录。严格看，帮助文本不是「数据」，`leeway help` 走 stdout 与「stdout 只输出数据」有张力；是否调整属行为变更，不在本次建议内。
8. **`leeway use` 非 JSON 时把 `switched … to account … (wrote …)` 打到 stdout**（cli.go:474-478，英文）：同样是「stdout 只输出数据」的张力点，且它是英文。只记录。
9. **`status.go` 的表头 `PROVIDER / ACCOUNT / CREDENTIAL / STATUS / EXPIRES` 与状态值 `ok` / `not logged in` / `not configured` / `expired …` 全是英文**：它们随表格走 stdout，我按「数据列名」处理，未列入中文改写；是否中文化需要产品决定（中文化会影响解析表格的脚本）。
10. **同位置的英文兄弟文案**：`unknown command %q`（cli.go:60）、`unexpected argument %q`（cli.go:55、flags.go:75）、`unknown provider %q`（login.go:59/96、logout.go:26）、`empty key; aborting`（login.go:51、auth.go:215）、`saved manual cookie header for %q`（cli.go:499）、`imported %d cookies …`（cli.go:541）、`cookie cache … is fresh … use --force`（cli.go:516）、`warning: --account binding requires --profile …`（cli.go:535）、`refuse to delete %s (not a leeway session dir)`（logout.go:57）、`switched … to account …`（cli.go:474）。它们面向用户但不是中文，严格不在本次范围；是否统一中文化需产品决定。其中 `refuse to delete` 与紧邻的中文成功提示最不协调。
11. **命令名是否要中文化**：硬约束 4 说「名称 2–4 个汉字」，但 `usage/login/logout/status/use/help` 是 CLI 契约（AGENTS.md 明确），我按「只改说明、不改命令名」处理；若作者希望给中文别名，那是行为变更，需另开任务。
12. **`auth.go:175` 与 `auth.go:244` 两种拼接**：前者 `"查额度：leeway usage grok"+suffixFlag(name)`，后者 `suffixAccount("查额度：leeway usage "+providerID, account)`。我建议统一成一种并统一用「看用量」，但不确定是否有意区分。
13. **`config.go` 与 `status.go` 没有中文用户文案**：`config.go` 的错误都是裸 `error`（如 `os.ReadFile` 的结果），经 `fmt.Fprintln(os.Stderr, "config:", err)` 打出，前缀是英文。本次未改；是否给配置错误加中文说明（例如「配置文件读不了，检查 ~/.config/leeway/config.json 权限」）需要产品决定。

---

## 五、这一块的文案问题概括

CLI 这一侧的文案是「实现者写给实现者」的：帮助文本把 provider 叫「家」、account 叫「号」、把备份叫「快照」、把切换叫「切号」、把账号表叫「台账」、把走势叫「点阵」，还把 `state.vscdb`、`keychain` 服务名、`LEEWAY_CURSOR_*` 开关、`targets` 字段这些内部名词放在 `leeway use` 的第一屏——一个只想「换个号看看额度」的人，扫一眼 `leeway help use` 找不到该敲哪条命令。错误提示则是另一个极端：`leeway use` 的六条失败里只有两条给了可照抄的下一步，其余（账号不存在、找不到会话库、备份失败、写入失败、找不到可写位置）只说「错了」，用户卡在原地；而且错误前缀中英混排（`usage:` / `config:` / `save key:` / `read key:` / `write session:` 对中文正文），同一类错误看起来不像一类。句式上也缺「重复与对齐」：`rootHelp` 里只有 `usage` 和 `login` 带参数行，`logout`/`status`/`use` 都不带；成功提示里「清除 / 移除 / 删除」「查额度 / 查用量」「私有登录 / 会话 / 令牌」各说各的。真正值得当模板的是 `cursor_ide.go:17` 和 `auth.go:39/204`：先说原因，再给一个能照抄的动作或链接——这三条应该成为全 CLI 错误文案的基准句式。所有问题都只在文字层，命令名、参数、退出码与「stdout 只出数据、日志和错误走 stderr」的通道契约本次一律未动，发现的通道张力（`leeway help` 走 stdout、`use` 的人类可读结果走 stdout）只做了记录。
