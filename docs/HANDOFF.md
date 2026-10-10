# leeway 界面品质交接（2026-10 会话）

这份文档交给下一个会话：质量目标、当前状态、已知问题、优化方向、可参考的项目与代码、验证方法。
读完这份 + `docs/GUI.md` 就能接手。**它是一份工作状态文档，不是永久契约**——目标达成后可以删掉。

---

## 1. 质量目标

维护者定的标准（原话）：

> 作为品质标准，目标是达到 awwwards、webby awards、FWA 能获奖的品质，通过自我检查，反复提升品质，
> 直到达到那个品质标准为止。

这条标准在会话里落成了三条可执行的做法，**下一个会话必须继续遵守**：

1. **先量后改**。任何「这里不好看」的判断都要先变成数字：对比度、线密度、像素颜色、行宽。
   本轮里我三次凭印象下结论，三次被测量推翻（见 §5 的教训）。
2. **对标真实项目，不靠感觉**。语料是 13 个真实用 MyGo 写的开源应用（§6），每个结论都要能
   指到 `文件:行号`。
3. **改完要能证伪**。测试要么用像素证明，要么用「把旧值放回去它会失败」证明有牙（§7）。

---

## 2. 当前状态（已提交）

```
1451e0d 状态层：卡片悬停原来只有 1.02:1，读者看不见
fba339c 把剩下的装饰线也收掉：页标题下、首次运行每行、账号之间、条的一圈
bacc6f8 卡片不再是一圈描边加一条 3px 凹槽：一块面，靠填充差和投影分开
3f02658 首次运行：详情 是画出来的控件，并挪到动作左边
3848559 侧栏写每家还剩多少，主角卡的读数收回数字那一行
23f1f08 mygo 升到 v0.4.0            ← 本会话之前
```

`gofmt -l internal/ cmd/` 干净，`go build ./...`、`go vet ./...`、`go test -count=1 ./...` 全绿。

### 2.1 量出来的结果（同一把尺子：整幅宽度 50% 以上的 1px 暗线）

| 页面 | 会话开始时 | 现在 |
|---|---|---|
| 总览 | 7 横 + 3 竖 | **2 横 + 0 竖** |
| 首次运行 | 7 横 | **0** |
| provider | 5 横 | **2 横 + 0 竖** |
| 历史 | 6 横 | 5 横 + 0 竖 |
| 设置 | 2 横 | 2 横 + 0 竖 |
| **合计** | **24 横 + 3 竖** | **11 横 + 0 竖** |

总览每千像素从 **14 条降到 3 条**。剩下 11 条逐条看过颜色：3 条是图上数据（色 107/212，
说明文字点名的那条线）、4 条是列表行线、4 条是条的底槽上沿（填充边界，不是画的线）。
**装饰性的框和线已经没有**。

### 2.2 设计系统现状（`internal/gui/theme.go`）

| 层 | 值 | 依据 |
|---|---|---|
| 画布 `canvas` | `#f2f2f6`，顶部 `canvasTop` 带强调色渐变 | — |
| 卡片 `card` | 白 82%（深色：白 5.5%） | 压在画布上 **1.10:1**（深 1.13:1）；motrix 是 1.09:1 |
| 卡片悬停 `cardHi` | `card.Mix(t.Text, stateHover)` | meiro `m3/theme.go:139` |
| 状态层 | `stateHover 0.08` / `statePress 0.10` | 同上，全语料唯一一张表 |
| 分组细线 `hairline` | 10% 墨（≈1.3:1） | 只分组，不是边界 |
| 表格分隔 `divider` | 黑 44%（浅 3.2 / 深 3.3:1） | MintUI：行间一条线，**表格周围一条都没有** |
| 有边的面板 `border` | 黑 16% | 只剩弹窗、设置行、菜单在用 |
| 条底槽 `track` | 黑 10% | 条没有描边（§4.1） |
| 字阶 | 40 / 25 / 20 / 16 / **13 正文** / **10.5 caption** | 每级 ≥1.2×；caption 比 motrix 的 11 还小 |
| 间距 | `space1..space6 = 4/8/12/16/24/32` | 所有布局间距都从这一梯取 |
| 圆角 | `radiusControl 10` / `radiusTray 18` / `radiusCapsule 999` | 卡片只有一层，`radiusInner`/`cardTrayPad` 已删 |

**已删除、不要加回来**：`bezel`、`bezelHi`、`borderHi`（托盘的遗留）、`trackEdge`（条的描边）、
`cardTrayPad`、`radiusInner`。卡片现在是**一块面 + 一条柔和投影**，不是「托盘里嵌一块板」。

---

## 3. 已知问题（按优先级）

### P1 — 按下状态用 `DrawOver`，会压暗文字
`internal/gui/page_overview.go:372` 的 `openCard` 用 `e.DrawOver(...)` 画按下层。MyGo 的
`DrawOver` 设的是 `paintAfterFn`（`ui/element.go:1099`），**画在子元素之上**，所以按下时
文字也被压暗 10%。meiro 的状态层画在内容**之下**。
修法：把按下也做成颜色（`cardPress = card.Mix(t.Text, statePress)`），在 `shellSurface`
里和 `cardHi` 一起管；`shellSurface` 需要知道按下状态（`surface.Pressed()` 是否在子元素上
成立要先验证，别假设）。

### P1 — 账号行的常驻按钮

> ⚠️ 本节的行号来自两个研究子代理读源码的结论，**我只逐条复核了 §6 表里的那些**。
> 引用前请自己打开文件确认；我已经知道它们至少压平过一次子目录。

`internal/gui/page_overview.go` 的 `overviewAccount` 每个账号常驻「刷新」「登录」两个按钮
（加上「详情」共三个控件）。语料里的做法不一致，需要判断而不是照搬：
- motrix：行操作走**右键菜单**（`downloads.go:415`），行内没有常驻按钮；
- meiro：共用的 `itemMenu`，每项一个按钮**或**右键任意处打开（`itemmenu.go:6-52`）；
- Yeek：行内**没有**按钮，侧栏级一个菜单 + 表头行 kebab（`view.go:328,1002`）；
- codex-monitor：**有**常驻按钮（`internal/nativeapp/settings.go:47,88,157`，这条我复核过）。
mine-D 的原话：「Nothing in either appears only on hover.」——所以 **hover 才出现是最差的方案**，
要么常驻 kebab，要么右键。

### P2 — 历史页还有 5 条线
2 条是图上的数据线（保留，说明文字点名它们）、3 条是行线（motrix `downloads.go:346`
就是单边底线，保留合理）。如果要再收，只能收行线，且要同步改两个把「线」当卡片探测器的
测试（`TestASingleHistoryTraySpansTheMeasure`、`TestTheHistoryCardsAreTheSameWidth`，
它们现在找 `p.hairline`）。

### P2 — 滚动边缘未做
MyGo 有 `glass.ScrollEdge`（`plugins/glass/edge.go`，macOS 26 的滚动边缘效果，用法见该文件
注释），**我们和 motrix 都没用**。要不要用需要先判断：它压在纯色画布上是否成立（我们的窗口
不是原生毛玻璃）。13 个语料项目里 **0 个**调用 `glass.Glass`/`Material`（我们上一轮已把侧栏
的 glass 去掉）。

### P2 — 文档与依赖版本不一致
`AGENTS.md` / `CLAUDE.md` 的构建命令写 `mygo@v0.3.4`，`go.mod` 实际钉 `v0.4.0`。
v0.4.0 = v0.3.4 + `ui/webview.go` + `ui/textlines.go`，glass 插件 API 逐字节相同。

### P3 — 没有命令面板
motrix（`Ctrl K` 芯片）和 Yeek/MujicaUI 都有。我们有 ⌘R 刷新，没有全局跳转。属于**功能**，
不是品质缺陷。

### 待决的产品/清理问题
- `~/.cache/leeway/history.jsonl` 现在 200 条、5 天（约 40 条/天）——想做年历热力图基本是空的，
  要么先扩保留期，要么放弃热力图。
- `build/screenshots` 3.6M、`~/.cache/leeway/*.bak-20260917`（101K）——是否清理。

### ⚠️ 仓库里有另一个 agent 在同时工作
`AGENTS.md` 写明主模型 Grok 4.6 / 次模型 gpt-sol。它的未提交文件是
`docs/ICON.md`、`internal/gui/icons.go`、`resources/icon-glyph.py`——
**不要回退，提交时只 add 自己的路径**（本会话一直这么做）。不要动 `go.mod` / `go.sum`。

---

## 4. 优化方向

### 4.1 已经用参考项目定下来的五条（继续照此判断）
1. **卡片**：靠**填充差 + 间距**分开，不靠描边。motrix 完全不画边；codex-monitor 是唯一留边的，
   但它在浅色下的边是 `RGB(255,255,255).Alpha(.93)`——**高光，不是深色轮廓**；vitals 用默认主题
   的 `#d9d9de` 且**零卡片投影**。三家都没有「卡片里再嵌一条 3px 装饰带」。
   ⚠️ 一个真实分叉：三个**应用**都给卡片投影，四个**组件库**一处都不给（它们的投影全在浮层上）。
   我们选了「应用」那一派。
2. **导航栏**：**0/7 用 `ui.Sidebar`，0/7 用 glass，0/7 在侧栏与正文之间画线**。motrix 的侧栏
   直接坐在窗口渐变上，选中项是平涂 wash `RGBA(0,0,0,0.09)`——没有强调色条、没有加粗。
   我们的选中态实测是 (221,221,225)、1.23:1、**中性灰不带强调色**，已经对了。
3. **文案的间隔**：caption 是 9–11 DIP + `TextMuted`，值是 12–34 DIP，**靠字号和颜色分层**。
   间距比想象中紧：MintUI **0 DIP**、uix/meiro `Gap(2)`（meiro 甚至用 **−6 DIP 负边距**）、
   Yeek `Gap(4)`、motrix 6–8。我们的 caption 10.5 + `space1`(4) 已经在范围内。
4. **同族元素的分割**：线只花在**卡片内部同一族的行之间**，而且是**单边底边线**
   （motrix `downloads.go:346`、`notifications.go:25` 用 `BorderWidth(0,0,1,0)`），
   **从不画整圈**；codex-monitor 在账号行上一条线都不画，只用 `Gap(4)`（`settings.go:108,133`）。
   MintUI 的 `treetable` 默认给行画线（`ui/data/treetable.go:172`），但**拒绝外框**：「围着它一圈会说『这是个东西』，
   而这张表只是页面的一部分。」
5. **列表**：行操作要么右键菜单、要么 kebab，**没有一家只在 hover 出现**；没有网格线、没有表框。
   斑马纹只有 motrix 在下载列表里用（`downloads.go:411-412`），另外三家为 0，MintUI 还明确
   反对（`ui/files/collab.go:525`）。

### 4.2 还没采纳的语料手艺
- **`glass.ScrollEdge`**（§3 P2）。
- **MintUI 的预留标记位**：`bar = BorderWidth*2` = 每行预留 2 DIP，理由是「被选中时才长出标记的行，
  会把旁边的文字挤动」（`ui/data/paint.go:32`）。矩阵排序标/侧栏若要有标记，照这个做。
- **meiro 的状态层表**已采纳（§2.2），但只用在卡片悬停上——按钮、行、菜单还是各配各的。
- **图的绘制动画**（codex-monitor 的 `p.Clip` 拉入）：未核实我们有没有，值得看一眼
  `internal/gui/chart.go` 之类。

---

## 5. 本会话踩过的坑（省你时间）

1. **`sizeLabel` 是 10.5，`sizeBody` 是 13**。我把两者记反，提出过「caption 13→11」——是错的，
   caption 本来就是 10.5。
2. **`DrawOver` = `paintAfterFn`，画在子元素之上**（`ui/element.go:1099`）。任何「垫在内容下面」
   的层都不能用它。
3. **卡片悬停曾经只有 1.02:1**（`card` 0.82 → `cardHi` 0.98，压画布 2 个单位）。托盘细边还在时
   读者看到的是那条边，边一去掉悬停就没了。修法是状态层（`1451e0d`）。
4. **有几个测试是「用线当元素探测器」**：页面列宽靠标题下的横线找、历史卡宽靠卡片头部的线找。
   删线会连带改测试——**改探测器，不要放宽断言**。
5. **`ButtonBase(c).Label("x")` 画不出东西**（`ButtonBase` 是「没有外观的按钮」，`Label` 只喂
   无障碍）。要看得见就得给它一个文字子元素（`page_start.go` 的「详情」）。
6. **`ui.Tester` 只有 `Find`（首个匹配）/`Texts`/`HasText`**，没有 find-all。侧栏和正文出现同样
   文字时，`Find` 会命中侧栏那个（本会话为此刻意让侧栏说「剩 NN%」这个正文不出现的字符串）。
7. **打包时的 `__pycache__` 竞态**：`mygo build` 报 `stat .../resources/__pycache__` 是并发写者
   在遍历中删了目录，删掉该目录再构建即可。

---

## 6. 可参考的项目与代码

**注意 `/tmp` 会被清空。**下面是仓库地址，需要时重新 clone 到 `/tmp/ref/clones/<owner>__<repo>`：

| 项目 | 地址 | 为什么看它（行号已复核） |
|---|---|---|
| motrix-mygo | `https://github.com/missuo/motrix-mygo` | **卡片 = 填充 + 淡投影，零描边**：`app.go:361`（`Radius(14).Background(p.card).Shadow(0,1,3,0,p.shadow)`，无 `Border`）；caption `widgets.go:30`（11/600/+0.8）；行分隔是**单边底线** `BorderWidth(0,0,1,0)`：`downloads.go:346`、`notifications.go:25` |
| codex-monitor | `https://github.com/oysterhyd/codex-monitor` | **同类产品**（用量仪表盘）；卡片 `internal/nativeapp/style.go:98`，浅色下的边是**高光** `internal/nativeapp/style.go:44`（`RGB(255,255,255).Alpha(.93)`）；账号行只用 `Gap(4)`：`internal/nativeapp/settings.go:108,133`；常驻行内按钮确实存在：`settings.go:47,88,157` |
| vitals | `https://github.com/xbenduan/vitals` | 用**默认 MyGo 主题**的样本；卡片 `view.go:284`（`Radius(8).Background(t.Surface).Border(1,t.Border)`，**无投影**）；全文几乎不画线 |
| MintUI | `https://github.com/HycJack/MintUI` | 组件库；单一描边宽度 `ui/theme/theme.go:185`（`const BorderWidth float32 = 1`）；卡片必须是 Surface-on-Background 的理由 `ui/theme/theme.go:22`；条件性 footer 线 `ui/data/card.go:20-23`；行线但**拒绝外框** `ui/data/treetable.go:51-52,172`；每行预留 2 DIP 标记位 `ui/data/paint.go:32`；反对斑马纹 `ui/files/collab.go:525` |
| meiro | `https://github.com/elianiva/meiro` | **状态层表** `m3/theme.go:139`（`StateHover float32 = 0.08` 等）+ 合成 `m3/button.go:102` `StateFill` + `m3/color.go:299` `Layer`；卡片是纯填充、无边框无投影 `browse.go:199`；行操作共用菜单 `itemmenu.go` |
| uix | `https://github.com/smallnest/uix` | 侧栏是 `Width(260)` + `Background(t.Surface)` 的盒子、靠 16 DIP 间距分开、**不画分隔线**：`components/appshell/appshell.go:52` |
| Yeek | `https://github.com/tacshi/Yeek` | 侧栏分隔线是**一条边**（`BorderWidth(0,1,0,0)`）：`internal/desktop/view.go:312`；fork 里加了静止不画线的可拖分隔 `third_party/mygo-ui/split.go:24,126`（`quiet` 时直接 `return`） |
| 线密度对照 | `egoist/gorex`、`egoist/godiff`、`ZacharyZhang-NY/MujicaUI`、`RedwindA/gotify-desktop`、`sugarbearr/gogo-explorer` | gorex 0、godiff 1、meiro 0、uix 9（每千像素的满宽 1px 线） |

> 路径都相对**各仓库根目录**。⚠️ 我的两个研究子代理把子目录压平过（例如把
> `internal/nativeapp/style.go` 写成 `style.go`、把 `ui/data/treetable.go` 写成
> `data/table.go`），**上面这张表是我逐条打开文件复核过的**；如果新会话从别处看到旧的行号，
> 以这张表为准。

**MyGo 本身**（钉在 `v0.4.0`，模块缓存 `/Users/helix/go/pkg/mod/github.com/egoist/mygo@v0.4.0/`）：
- `ui/sidebar.go:43` 只画 `Background(t.Surface)`，**不画分隔线**。
- `ui/table.go:219` 自带表头分隔线（`coreDivider`）。
- - `plugins/glass/edge.go` `ScrollEdge`；`glass.go:127,138-139` `Glass` 的 rim 是 1 DIP、0.35 alpha、无投影。
- `ui/element.go:1096,1099` `Draw` / `DrawOver` 的语义。
- 项目自己的笔记：`docs/MYGO-NOTES.md`（改窗口布局/测试/构建前**先读**，尤其「百分比高度按父容器
  content box 解析」和「菜单用 `Tester.Menu()` 测」）。

**本会话留下的原始研究**（如果 `/tmp` 还在）：
`/tmp/mine-a/REPORT.md`（motrix/codex-monitor/vitals）、`/tmp/mine-b/report.md`、
`/tmp/mine-c/report.md`（motrix/codex-monitor/vitals，150 行）、
`/tmp/mine-d/report.md`（MintUI/Yeek/meiro/uix，84 行）。
参考截图 `/tmp/ref/shots/<owner>_<repo>/*.png`。

---

## 7. 怎么验证（照做）

```sh
# 常规四件套
gofmt -l internal/ cmd/
go build ./... && go vet ./... && go test -count=1 ./...

# 渲染快照（42 张 PNG，人眼过版式）
LEEWAY_GUI_SNAPSHOT=/tmp/shots go test -count=1 ./internal/gui/

# 真窗口截图（需要 GUI 会话）
LEEWAY_GUI_CAPTURE=/tmp/window.png open build/darwin-arm64/leeway.app

# 打包（AGENTS.md 写 v0.3.4，实际 go.mod 是 v0.4.0，用后者）
go run github.com/egoist/mygo/cmd/mygo@v0.4.0 build
```

**线密度尺子**（本会话所有「多少条线」的数字都用它，Python 在
`/Users/helix/.dsh/dsh-runtimes/dsh-primary-runtime/dependencies/python/bin/python3`，有 PIL + numpy）：

```python
import numpy as np
from PIL import Image
def count(path):
    a = np.asarray(Image.open(path).convert("RGB")).astype(float)
    L = a @ np.array([0.2126, 0.7152, 0.0722]); H, W = L.shape
    d = L[1:-1] - L[:-2]; e = L[1:-1] - L[2:]
    m = (d < -4) & (e < -4)          # 比上下都暗 4 个单位的像素 = 一条线的候选
    hs = (m.sum(axis=1) >= 0.5*W).sum()   # 横线：整幅宽度一半以上
    vs = (m.sum(axis=0) >= 0.5*H).sum()   # 竖线
    return hs, vs, (W*H)/1e6
```

**证明测试有牙**：把旧值放回去，跑那条测试，确认它以**正确的话**失败，再恢复。
本会话用这招验证过 `TestTheCardsHoverIsVisible`：

```
--- FAIL: TestTheCardsHoverIsVisible
    the card's hover is 1.02:1 from its rest ({255 255 255 255} vs {253 253 253 255}): nobody can see it
```

**对抗式审查**（`AGENTS.md` 要求，交付前必做）：列出最可能翻车的 3–5 点，改完再交；
不接受「看起来没问题」。

---

## 8. 硬约束（不要破坏）

- **CLI 的 stdout 只输出数据**，日志/错误走 stderr；`cmd/leeway` 的行为是契约，改动同步
  README 与 `leeway help`。
- 不打印、不复制、不提交密钥/cookie/token；界面只显示登录信息来源，不显示内容。
- 未经明确授权**不访问真实 provider HTTP**，不读本机 cookie / keychain。
- 窗口通过 `internal/*` 的导出 API 复用 CLI 的同一条路径，不要另起一套探测或配置逻辑。
- **最小充分改动**；只防 P0/强 P1；不加「以防万一」的校验、重试、兼容路径。
- 只 add 自己的路径提交（仓库里有另一个 agent）；不动 `go.mod` / `go.sum`。
- 本会话**审批提示已禁用**：不要设 `sandbox_permissions`；文件策略是 danger-full-access。
- 不要另起服务器替代 GUI。

---

## 9. 建议的下一步（按性价比）

1. **P1 按下状态改用状态层颜色**（`shellSurface` 同时管 hover 和 press），消掉 `DrawOver` 压暗文字。
2. **P1 账号行的动作**：定一个方案（常驻 kebab 或右键菜单），做之前先确认键盘可达性
   （现有测试 `TestTheCardOpensFromTheKeyboard` 是这类交互的样板）。
3. **P2 判断 `glass.ScrollEdge`** 在纯色画布上是否成立；不成立就明确记一笔「不做，因为……」。
4. **P2 统一状态层**：把按钮、行、菜单的悬停/按下都走 §2.2 那张表，删掉各组件自配的 alpha。
5. **P3 命令面板**（功能级，需先问维护者要不要）。
