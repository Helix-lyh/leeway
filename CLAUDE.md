# leeway 项目协作指南

`AGENTS.md` 是项目协作主文档，`CLAUDE.md` 同步共有项目指引；仅面向 Codex 的代码生成规则只保留在 `AGENTS.md`，其他共有内容修改时必须同步。

## 工作方法论

维护者本机约定（贡献者不必遵循）：主模型是 Grok 4.6，次模型是 gpt-sol。Grok 负责读现状、改文件、跑验证和收口；需要强推理、方案对照或对抗复核时再交给 gpt-sol。别家模型的接口参数、effort 档位、thinking-block 协议不要写进本仓库规则。

### 第一性原理

- 动手前先回到根本：这个任务到底要解决什么问题？别照搬惯例或“大家都这么做”。
- 把问题拆到最小、能验证的单元，一个个解决。
- 每个决定都说得出“为什么”，而不只是“怎么做”。

### 对抗式审查（交付前必做）

- 写完先切换成最挑剔的审查者，从逻辑漏洞、事实对不对、有没有更简单的做法这几个角度攻击自己。
- 主动列出最可能翻车的 3 到 5 个点，改完再交。
- 不接受“看起来没问题”，得拿出验证过的证据。

## 项目定位

leeway（余量·裕度，模块名 `leeway`）是 AI 编程 provider 用量 / 额度 / 重置时间的探测工具，复刻 CodexBar 的探测核心。CLI 优先：JSON 优先、stdout 只输出数据，方便 agent 直接调用；另有一个原生窗口界面（MyGo 的 `ui` 包，纯 Go 无 cgo）。当前实现只有窗口进程：关窗即退出，没有菜单栏常驻或后台守护。

支持 provider：**deepseek / kimi / grok / codex / cursor / opencode-go / commandcode**。名单以 README 当前列表为准，不要臆造未实现的家。

入口：`cmd/leeway/main.go`（CLI）、`cmd/leeway-gui/main.go`（窗口，MyGo 配置在 `mygo.json`）。探测在 `internal/provider/`，CLI 在 `internal/cli/`，窗口在 `internal/gui/`，JSON 契约在 `internal/model/`。架构见 `docs/DESIGN.md`，窗口见 `docs/GUI.md`，各家探测见 `docs/PROVIDER-FETCH.md`，图标见 `docs/ICON.md`。Go 版本以 `go.mod` 为准。

## 命令

```sh
go install ./cmd/leeway
go build -o leeway ./cmd/leeway && ./leeway usage

leeway help
leeway help login

leeway usage
leeway usage cursor work
leeway usage --json
leeway usage --last 24 cursor

leeway status
leeway login cursor
leeway login kimi
leeway logout cursor work
leeway use cursor work
leeway use grok home

go test ./...

go run github.com/egoist/mygo/cmd/mygo@v0.4.0 build      # 窗口产物：.app + .dmg
LEEWAY_GUI_SNAPSHOT=/tmp/shots go test ./internal/gui/    # 渲染快照，人眼过版式
LEEWAY_GUI_CAPTURE=/tmp/window.png open build/darwin-arm64/leeway.app   # 真窗口截图自检
```

CLI 与窗口共用 `internal/config`、`internal/fetch`、`internal/history`、`internal/auth`、`internal/switcher`；窗口自己的偏好放在 `~/.config/leeway/gui.json`，不要塞进 `config.json`（CLI 的 `Save()` 会抹掉不认识的字段）。

日常看 `leeway help`；某一条看 `leeway help login`（还有 usage / logout / status / use）。无子命令时往 stderr 打帮助并退出 2。`leeway login cursor/grok/codex/kimi` 走官方工具；其余粘贴 API 密钥。`leeway use` 把 cursor 写给本机所有已有的官方登录位置（cursor-agent 的 macOS keychain 项、Cursor IDE 的 `state.vscdb`、`~/.cursor/auth.json`；先退出 Cursor，`cursor-agent` 不算；缺的跳过，都没有才报错），grok 只回写完整官方 `~/.grok/auth.json`，kimi 登录文件写官方 kimi-code.json，codex 不写 `~/.codex`，其余写 DSH；不是切换 leeway 自己看哪个账号。`PROVIDER` / `ACCOUNT` 写在命令后面，也可写成 `--provider` / `--account`。近 N 次走势用 `usage --last`。

退出码：`0` 全部通过；`1` 至少一个 provider 报错或未配置；`2` 用法 / 配置错误。

窗口界面（`internal/gui`）的 MyGo 用法、坑与出处都在 `docs/MYGO-NOTES.md`：改窗口布局、测试或构建前先读它（尤其「百分比高度按父容器 content box 解析」与「菜单用 `Tester.Menu()` 测」两条）。

## 硬约束

- CLI 的 stdout 只输出数据（JSON 或表格）；日志与错误一律走 stderr。窗口界面不写 stdout。
- 不打印、不复制、不提交密钥、cookie、token、Authorization 或含真实凭据的配置；界面只显示登录信息来源，不显示登录信息内容。文档示例只用占位。
- 配置文件 `~/.config/leeway/config.json`（0600，可用 `LEEWAY_CONFIG` 覆盖）；缓存 `~/.cache/leeway/`（`LEEWAY_CACHE`）；窗口设置 `~/.config/leeway/gui.json`（0600）。凭据优先级：环境变量 → 配置文件。
- 未经明确授权，不访问真实 provider HTTP，也不读用户本机 cookie / keychain 做探测。
- 不要另起一套探测或配置逻辑：窗口通过 `internal/*` 的导出 API 复用 CLI 的同一条路径。
- `cmd/leeway` 的 CLI 行为是契约，改动要同步 README 与 `leeway help`。
