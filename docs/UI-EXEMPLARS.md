# mygo 生态：谁在用，以及他们的 UI 层怎么写的（2026-10-09）

> 目的：leeway 的窗口是 [MyGo](https://github.com/egoist/mygo) 的原生 `ui` 包画的。在动手提升
> 界面品质之前，先摸清生态里有哪些真实项目、他们的布局与样式是怎么落的，避免我们关起门来
> 自己发明一套。本文只记录**查得到出处**的东西，每条结论都能顺着链接与 `file:line` 复核。

## 0. 结论先说

- 生态**真实但极新**：34 个仓库确证 import 了 `github.com/egoist/mygo/ui`（`go.mod` 钉版本 +
  `.go` 里真 import，本地 clone 后 grep 出来的），另有 4 个只把 MyGo 当 webview 宿主。
  其中约 490 颗星里有 452 颗在 egoist 自己的三个 app 上（lorca 251 / godiff 110 / gorex 91）；
  第三方最高是 `OUBIGFA/WinTray`（43★），第三方**原生 UI** 最高是 `ZacharyZhang-NY/MujicaUI`（24★）。
  几乎全部仓库创建于 2026-10-04 至 10-09，也就是几天之内。
- 生态里**没有**「谁在用」的官方清单：docs 站点没有 showcase，pkg.go.dev 的 imported-by 列表可用但
  计数器是坏的（三处都写 `Known importers: 0`），GitHub 的 dependents 页面是客户端渲染，抓不到。
  所以清单是靠代码检索 + fork/PR/issue 的作者 + egoist 自己的仓库列表拼出来的。
- 值得学的不是代码量，是**设计取舍**。最大的两个组件库（MujicaUI 1207 个 UI 文件、MintUI 305 个）
  提交历史里是成百上千条 `Merge branch 'worktree-agent-*'` 与 `WIP components (as built)`，
  基本是 agent 生成物；`uix`（227 文件）与 `meiro`、`kopi` 这些手写的小项目反而更干净。

## 1. 怎么找的（可复跑）

```sh
gh search code 'github.com/egoist/mygo v0' --limit 100      # 最有效的一条
gh search code 'ui.NewTester'                                # 找真正画原生 UI 的
gh search code 'mygo.json'                                   # 找用 mygo build 打包的
gh repo list egoist --limit 300 --json name,description      # 他自己的 300 个仓库里只有 4 个是 Go
# 然后逐个 shallow clone 到 /tmp/mygo-research/clones/，grep go.mod + import
```

**已知拿不到的**（别再试）：`github.com/egoist/mygo/network/dependents`（React 客户端渲染，
190KB HTML 里 0 行数据，`.json` 端点 406）；pkg.go.dev 的 `Known importers` 计数（坏）；
`gh search code 'NewStyle('`（110 条几乎全是 lipgloss 误报）；`gh search repos 'mygo ui'`（空）。

## 2. 采用者清单

原生 UI（import `…/mygo/ui`），按 UI 代码量排序，完整 34 个见下表节选：

| 仓库 | ★ | UI 代码行 | mygo | 是什么 |
|---|---|---|---|---|
| [ZacharyZhang-NY/MujicaUI](https://github.com/ZacharyZhang-NY/MujicaUI) | 24 | 1207 | v0.2.10 | 组件库，100 个组件、2 主题 × 3 密度 |
| [HycJack/MintUI](https://github.com/HycJack/MintUI) | 0 | 305 | v0.2.16 | 组件库 + gallery |
| [smallnest/uix](https://github.com/smallnest/uix) | 4 | 227 | v0.3.3 | shadcn 风格注册表，~78 组件 + 18 个 demo |
| [saanuregh/dgopher](https://github.com/saanuregh/dgopher) | 0 | 62 | v0.3.3 | 真 app：数据库客户端 |
| [egoist/lorca](https://github.com/egoist/lorca) `desktop/` | 251 | 53 | v0.3.4 | 真 app：Telegram 式 agent 聊天 |
| [kkkk2323/droi](https://github.com/kkkk2323/droi) | 9 | 43 | v0.3.3 | 真 app：Factory Droid 桌面端 |
| [KevinXC5/leafmark](https://github.com/KevinXC5/leafmark) | 0 | 37 | v0.2.15 | 真 app：Markdown 编辑器 |
| [zucchiniEvader/kopi](https://github.com/zucchiniEvader/kopi) | 0 | 35 | v0.2.12 | 真 app：代码编辑器 + Java LSP |
| [oysterhyd/codex-monitor](https://github.com/oysterhyd/codex-monitor) | 2 | 25 | v0.2.15 | 真 app：**Codex 额度监控（和我们最像）** |
| [elianiva/meiro](https://github.com/elianiva/meiro) | 0 | 25 | v0.3.3 | 实验性 YouTube Music 客户端（Material 3） |
| [egoist/godiff](https://github.com/egoist/godiff) | 110 | 21 | v0.3.4 | 真 app：原生 diff 查看器 |
| [egoist/gorex](https://github.com/egoist/gorex) | 91 | 9 | v0.3.5 | 真 app：Rex 终端 |

其余（WinTray 43★、jfut/ssh-keyselect 8★、xiangzhuan 3★、MQTT-Desktop、LeagueAkari-MyGo、
pomodoro、xiangqi_go、keyboardlock、calc、image-min-go、dji-mic-rx、aster、minicode、
agent-ebpf-filter、texture-harmonizer、qwen-image、gotify-desktop、Yeek、video、
mygo-breeze-ui、mygo-agent、HycJack/mygo-agent 等）代码量都在 30 行 UI 以下或只是壳，
不值得照抄。只把 MyGo 当 webview 宿主的有 `zkeq/vibe-cook`、`trae`、`VoiceMorph` 等。

## 3. 五个最值得读的实现

**MujicaUI — 令牌投射（`core/theme.go:81-108`、`theme/tokens.go:92-127`）**
每帧用应用自己的 22 个语义字段**重建**一个 `&ui.Theme{}` 再 `SetTheme`，于是滚动条、选区、
焦点环这些**系统控件也继承同一套色**；`HighContrast` 只是 `TextMuted = Text`、`Border = ControlBorder`；
密度是乘数（`Unit()` 3/4/5 DIP）不是尺寸，配 `SpaceXS…SpaceXXL` 名字阶梯；动效 120ms 状态 / 160ms 弹层。
它的 `theme.Contrast`（WCAG 比值）是**生产代码**，用在取色器和图表里，不是测试工具。

**meiro — Material 3 用 Oklch 实现（`m3/theme.go`、`m3/color.go:256-288`）**
一个种子色 → 6 条色调曲线 → 角色方案；Tone 到 Oklab 明度走 CIE L\* 公式，超 sRGB 的彩度用 22 步
二分找边界。角色映射：Primary/Error 亮 40 暗 80，On\* 100/20，Container 90/30，Surface 98/6。
状态层 hover .08 / focus .10 / press .10；**高度层级只靠阴影不靠表面染色**（两层黑，alpha .30k 与 .15k）。

**kopi — 与模式无关的中性色（`theme.go:61-100`）**
hover/pill 用 `RGBA(127,127,127,.11)`（亮）/`.16`（暗），muted 用 `.48`、卡片边 `0.10` 两个模式**共用同一个值**；
macOS chrome 用 `TitleBarHiddenInset`、`TitleBarHeight 40`、红绿灯 `{14,13}`、`VibrancySidebar`，
工具栏内边距按实测的红绿灯位置算（`bar.Left+8`）。

**uix linechart — 布局负责命中、画笔负责渲染（`components/linechart/linechart.go:139-199`）**
一行 `Grow(1)` 的隐形盒子当等宽分区，`col.Hovered()` 拿索引；`Draw` 里画网格、面积、线、标记。
数据最大值留 **10% 顶部余量**，**4 条网格线**在 `H*i/3`、1px、主题 border 色；当前线实线 2.5、
上一条虚线 2.0（Text 的 30%，5/5 相位跨段延续）；渐变面积用**4 条带** alpha .30/.225/.15/.075、
每 2 DIP 一档伪造；悬停是 r7 光晕（.25）垫在 r4 圆点下。**没有 tooltip**，悬停改写页头数字。

**lorca / godiff / kopi — 密集应用的骨架**
侧栏宽度、栏高写成常量而不是按比例（理由写得很清楚：会随窗口变窄的侧栏会截断自己的标签）；
卡片标题与正文只差一档字号加一个权重；图标一律 SVG、按主题换色、16×16 起步。

## 4. 我们采纳了什么

| 技法 | 出处 | 在 leeway 的落点 |
|---|---|---|
| 令牌投射进 `ui.Theme` | MujicaUI | `theme.go`：`t.TextMuted`、`t.AccentText` 由我们定，而不是吃 MyGo 默认 |
| 对比度当成可断言的不变量 | MujicaUI `theme/contrast.go` | 新增 `contrast_test.go`：tone 色、次要文字、胶囊标签、按钮字一律 ≥4.5:1 |
| 密度/间距是乘数 | MujicaUI | `space1..space6`（4/8/12/16/24/32）、`radiusControl/Inner/Tray/Capsule` |
| 短动效词表 + 单一减弱动效闸门 | MujicaUI、MintUI | `stateMotion`(140ms 状态) / `panelMotion`(240ms 面板)；`ReduceMotion` 由 MyGo 短路 |
| 布局命中 / 画笔渲染分离 | uix linechart | 走势图重画（2px 描边、渐变面积、端点光晕、4 条网格、10% 余量） |
| 高度层级只靠阴影 | meiro | 已有 `shadowSoft` 双层阴影，未引入表面染色 |

**没采纳的**：Material 3 的整条色调曲线（我们是 Oklch 手调语义色，规模不值得引引擎）；
meiro 的弹簧动效（MyGo 没有 spring，`Animate` 只有缓动函数）；MujicaUI 的「每帧重建整个
`ui.Theme`」（我们只改需要改的字段，`paletteOf` 是纯函数）。

## 5. 闭源成品标杆

[Pulse](https://pulse.egoist.dev/)（egoist，MyGo，无公开源码）是比开源语料更「做完」的一极：
深色侧栏蓝 pill、三层灰无描边卡、点阵量图。规则与实机截图落在仓库内
[`PULSE-AESTHETIC.md`](PULSE-AESTHETIC.md)、[`ref/pulse/`](ref/pulse/)，不依赖 `/tmp`。

## 6. 复核

```sh
ls /tmp/mygo-research/clones/            # 44 个 shallow clone
/tmp/mygo-research/github/REPORT.md      # 检索方法日志 + 逐仓库证据
/tmp/mygo-research/framework/REPORT.md   # mygo v0.3.4 能力矩阵与「做不到」清单
/tmp/mygo-research/mined/REPORT.md       # 上述四个实现的具体技法摘录
/tmp/mygo-x/report.md                    # x.com 侧的采用者与演示（经 grok 无头检索）
```

X 侧补充：作者在 X 上是 `@localhost_5173`（备用 `@localhost_4173`），2026-10-03 的发布帖称它是
「GPUI 和 Tauri 生了个孩子，但是 Go 做的」。帖子里出现且能对上仓库的有 gorex、godiff、
`examples/effort-slider`、skills-reader、mygo-agent、lorca；只在帖子里出现、源码无法核实的
（Moshi Go、Clips 等）不记入本清单。
