# mygo 使用笔记（leeway 窗口）

> 这份文档是 leeway 的窗口界面（`internal/gui/`，非测试代码约 5700 行、含测试约 7200 行，纯 Go 无 cgo）在
> [github.com/egoist/mygo](https://github.com/egoist/mygo) 的 `ui` 包上踩出来的经验：
> 布局语义、测试与版式验证、组件与主题写法、构建分发。**每条都标了出处**——mygo 源码/文档的位置，
> 或本仓库的 `文件:行`。现象后面带"实测"的，是在 v0.2.16 上跑最小用例量出来、
> 并用同一套探针对照 v0.3.4 逐条复核一致的数字（mygo 行号已按 v0.3.4 重编）。
>
> **行号怎么读**：mygo 的行号对 `v0.3.4` 的发布源码，稳定；本仓库的行号是**并行开发中某一刻的快照**
> （核对于 2026-10-09），会随重构漂——所以每个本仓库引用旁边都给了符号名（函数/常量/测试名），
> **行号只作定位起点，以符号名为准**。
>
> 这里只写"mygo 怎么用"。窗口自己的设计见 [GUI.md](GUI.md)，架构见 [DESIGN.md](DESIGN.md)。

## 0. 版本，以及去哪儿看

- 版本钉在 `go.mod`：`github.com/egoist/mygo v0.3.4`。
- **先读文档，再读源码。** mygo 的文档就在模块里：`docs/ui/*.md` 有 71 篇、约 4100 行，一个控件一篇，
  另有 `layout.md`（尺寸/对齐/间距）、`styling.md`、`testing.md`、`transitions.md`、`rendering.md`、
  `views.md`、`windows.md`；模块根还有 `docs/native.md`（对话框、剪贴板、通知、托盘）、`docs/windows.md`、
  `docs/configuration.md`、`docs/distribution.md`。`examples/gallery/main.go` 把控件排成一页。
  这份笔记里的坑，一半以上在 `docs/ui/` 里本来就有答案，只是容易先去看源码。
- 再看源码：

  ```sh
  go mod download github.com/egoist/mygo        # 缓存里没有就先拉一次
  M=$(go env GOMODCACHE)/github.com/egoist/mygo@v0.3.4
  ls "$M/docs/ui"
  go doc github.com/egoist/mygo/ui.Element      # 或 go doc -all github.com/egoist/mygo/ui
  ```

  本文引用 mygo 时用相对 `$M` 的路径，例如 `ui/layout.go:513`；**不带目录的 `xxx.go` 是模块根
  （包 `mygo`，如 `window.go`、`window_state.go`、`modules.go`），别和 `ui/xxx.go` 混**
  （`CapturePage` 在根 `window.go:1085`，`ui/window.go` 只有 675 行）。`internal/text/*.go` 是 mygo 自己的
  内部包，只用来解释行为，**应用不能 import**。
- 别在临时模块里用 `go mod tidy` 探 API：它会解析到比钉住的更新的版本，看到的行为未必一样。
  临时模块要显式 `require github.com/egoist/mygo v0.3.4`。
- 源码入口：`ui/element.go`（元素与 `edges`）、`ui/layout.go`（flex 布局与百分比解析）、
  `ui/context.go`（builder、主题、`Size`/`TitleBar`/`AnimationFrame`）、`ui/menu.go` + `ui/widgets.go`（菜单）、
  `ui/headless.go`（`Tester`/`Render`）、`ui/transition.go`（过渡）、`ui/svg.go` + `ui/bitmap.go`（图标）。

## 1. 布局与尺寸

### 1.1 百分比高度按"父容器的 content box"解析；父容器高度不定时静默退化成 auto

这是本项目最贵的一个坑。规则本身很干净（`docs/ui/layout.md:33`–`36`：`Fill`/`FillWidth`/`FillHeight`
take the parent's whole content box），而且**是 content box，不是 border box**——实测：300 宽的父容器加
`Padding(20)` 后，子元素 `FillWidth()` 的红色像素是 x=20..279（宽 260 = 300−2×20）；
200 高的父容器加 `Padding(20)` 后，`HeightPercent(100)` 是 y=20..179（高 160）。

难在**父容器的 content box 常常不是你心里那个**。实测（窗口 400×300、标题栏 54、页面内容 900 高，
`LAST` 是页面最后一行，滚到底后量它的 bottom）：

| 父容器的状况 | `FillHeight()` 解析成 | 后果 |
|---|---|---|
| 确定，父容器是根 `Column(c).Fill()`（主轴 = 高度） | 父容器的 content 高度 | 解析值可能比可用空间大，但 flex 的 shrink 会把它收回来：`标题栏 + Scroll.FillHeight()` 滚到底 `LAST` bottom = **300 ✓**（再加 `Grow(1)` 一样） |
| 确定，但父容器自己就比可视区高一个标题栏（`Row.Height(winH)`） | 父容器的 content 高度 = 整窗 | 视口伸出窗口下沿，页面最后一截滚不到：`LAST` bottom = **338.7 ✗**（比可视区低 54px，正好一个标题栏） |
| **auto**（`Row` 没写 `Height`） | 解析不出来 → 按 auto（内容高度） | 滚动容器长到内容高度，**根本滚不动**，整页摊开：`LAST` bottom = **954 ✗** |
| `ui.Scroll` 内部（内容高度按定义是 `inf`） | 同上 → auto | 同上 |

源码依据：`ui/element.go:56`–`66`（`length.resolve(base)`，`unitPercent` 在 `base < 0` 时返回 false）、
`ui/layout.go:61`–`66`（`base(inf) = -1`）、`ui/layout.go:455`–`457`（`flagScrollY` 把内容高度设成 `inf`）、
`ui/layout.go:513`–`565`（flex 里子元素的 base 是 `c.height.resolve(base(ch))`，`ch` 是父容器的 content 高度）。

**正确写法：要"剩下的高度"就自己算，或者让 flex 分。** 本仓库的修法（`internal/gui/view.go:128`–`156`）：

```go
ui.Column(c).Fill().Children(func() {                 // 根 Column：高度 = 窗口，确定
    a.titleBar(c, p)                                   // 固定 titleBarHeight(c)
    // The row below the titlebar is given its height outright: a scroll inside
    // it clamps against its own box, and a percentage height would hand it the
    // window's instead, which leaves the end of a long page unreachable.
    _, winH := c.Size()                                // internal/gui/view.go:137
    ui.Row(c).Height(winH - a.titleBarHeight(c)).Children(func() {   // :138 显式算出来
        ui.Sidebar(c, &a.page, func() { a.sidebarItems(c, p) }).
            Width(236).FillHeight().Padding(12, 10).Material(glass.Glass{})
        ui.Scroll(c).Grow(1).FillHeight().Padding(26, 32, 40, 32).
            Children(func() {
                ui.Column(c).FillWidth().Gap(22).Transition(pageMotion).
                    Children(func() { a.pageView(c, p) })
            })
    })
})
```

`titleBarHeight` 取"自己内容高度或桌面要求的更多"（`internal/gui/view.go:173`–`182`，`titleBarMin = 54`），
来源是 `c.TitleBar()`（`ui/api_gen.go:420`，`docs/ui/windows.md` 的 Title bars 一节）。

三条可复用的结论：

1. 在 **Column/Box** 里，`Grow(1)` 就够（实测根 Column 里标题栏 + `Scroll.Grow(1)`，滚到底 bottom = 300，正确）。
2. 在 **Row** 里，`Grow(1)` 管的是**宽度**；子元素的**高度**要靠 `FillHeight()` 或 `AlignItems(ui.Stretch)`，
   而这两者都需要 Row 自己有确定高度。实测：`Row.Height(winH-54)` + `Scroll.FillHeight()` → 300 ✓；
   同一个 Row 里 `Scroll` 不写高度 → 619.3 ✗；改成 `AlignItems(ui.Stretch)` → 300 ✓。
3. 回归测试盯住这条：`internal/gui/gui_test.go:934`–`967`（`TestTheEndOfALongPageIsReachable`
   滚到底后最后一行必须落在滚动区自己的下边距之上）。

### 1.2 `AlignSelf(Stretch)` 覆盖父 Row 的 `AlignItems`，但只在交叉轴上

多账号矩阵列间那条贯通竖线就是它（现在那条线由 `page_matrix.go` 的 `Dividers(1, p.divider)` 画，见 1.4；这个手法本身仍然有效）：

```go
// columnRule 是当时矩阵列间那条竖线的写法（后来矩阵改用 Dividers，这个 helper 已删）
// columnRule separates two accounts in the matrix. AlignSelf(Stretch) makes it
// as tall as the row's other children, so the columns read as columns down the
// whole matrix and not only inside one cell.
func columnRule(c *ui.Context, p palette) ui.Element {
    return ui.Box(c).Width(1).Background(p.divider).AlignSelf(ui.Stretch)
}
```

实测：`Row.Height(120).AlignItems(ui.Start)` 里放一个 `Width(1)` 的 Box（没写高度），
加 `AlignSelf(ui.Stretch)` 后蓝色像素覆盖 **0..119**（整行高 120），不加则**一个蓝色像素都没有**
——它高度是 0，整条竖线根本看不见。依据：`ui/api_gen.go:2139`–`2144`（`AlignItems`/`AlignSelf`）、
`ui/layout.go:810`–`820`
（`selfAlign` 先看元素自己的 `self`，再退回父容器的 `align`；`stretches` 对 `kindIcon` 和交叉轴上有
auto margin 的元素返回 false——**竖线要用 Box，别用 Icon**）。

补三条边界：

- **Stretch 只作用于父容器主轴之外的那一轴**：Row 里是高度，Column 里是宽度。
  在 Column 里，`AlignSelf(Stretch)` 管的是宽度，高度还是元素自己写的那个值（没写就是 0）。
- Row 的高度是 auto 时，Stretch 子元素拿的是**这一行的行高**，也就是最高的那个兄弟的高度
  （实测：兄弟 60 高 → Stretch 的线 0..59）。所以"空预测行没有多余高度"不是 Stretch 失效，
  而是那一行没有任何元素有内在高度，行高本来就是 0。
- 元素的高度有确定值（写了 `Height`）时，Stretch 不覆盖它（`ui/layout.go:637`–`643`：
  `if _, ok := c.height.resolve(base(ch)); !ok` 才拉伸）。

### 1.3 `Gap` 加在**每一对相邻子元素**之间；插一个 1px 分隔线要多付两个 gap

`Gap(v)` 同时设 `gapX`/`gapY`（`ui/api_gen.go:3070`），flex 布局按主轴取用（`ui/layout.go:516`–`522`）。
实测（窗口 400、`Gap(14)`、三列 `Grow(1).MinWidth(0)`）：

| 写法 | 三列起点 | 每列宽 |
|---|---|---|
| 三列直接排 | `[0 138 276]` | 124 |
| 每两列之间插 1px 的 `columnRule` | `[0 143 286]` | 114 |

也就是**每列窄了 10px ≈ (2×14 + 1)/3**。矩阵仍然"一屏装得下"，靠的是
`internal/gui/gui_test.go:504`–`530` 那条数 12 个 `后重置` 的测试兜着。

**更省的替代做法**：容器自带的 `Dividers(width, color)` 在**已有的 gap 中间**画线，不占布局宽度
（`ui/api_gen.go:2680`–`2686`，`docs/ui/layout.md:23`–`25`）。实测同样三列加 `Dividers(1, …)`：
列起点仍是 `[0 138 276]`，线画在 x=131/269（显式子元素是 128/271）。代价是线的位置由 gap 决定，
不能像 `columnRule` 那样自己控制；收益是每列多 10px。**下次做多列矩阵可以先用 `Dividers`。**

### 1.4 `Grow(1)` 要配 `MinWidth(0)` 才是"等宽且可压缩"的列

`ui/layout.go:554`–`564`：Row 子元素的自动最小宽度是它的 **min-content**（`intrinsic(c, false)`），
除非它滚动或者 `ClipX`。列里有一个不可断的长词（或固定宽度的内容）时，这个自动最小值会让 flex
分配不均。实测：窗口 300、三列 `Grow(1)`、无 `Gap`、每列一个不可断的长词，量每列第一行文字的位置和宽度：

```
不加 MinWidth(0)：M0 x=0 w=183.3   M1 x=183.3 w=116.7   M2 x=366.7 w=0.0   ← 最后一列被顶出窗口外
加了 MinWidth(0)：M0 x=0 w=100.0   M1 x=100   w=100.0   M2 x=200   w=100.0 ← 等宽
```

也就是**不是"撑开"而是"被顶出去"**：窗口只有 300 宽，前两列按各自的 min-content 抢走 300，第三列
起点落在 366.7，整列看不见。

本仓库每一列都写 `Grow(1).MinWidth(0)`：`internal/gui/page_matrix.go:65`、`:78`。
注意 `MinWidth(0)` 不阻止溢出，只是允许压缩；真正把内容收进去的是 `SingleLine()`/`MaxLines(n)`
（见 2.4 与坑 14）。

### 1.5 文字之间的空隙靠 `Gap`，不靠空格

两个 `ui.Text` 放进没设 `Gap` 的 Row 会紧挨着渲染（实测 `"5-hour"` 的右边缘 41.58 = `"已用"` 的左边缘 41.58，
中间 0px），看上去就是 `5-hour已用`。给容器 `Gap(6)`（`internal/gui/page_matrix.go:92`、`:100`）。

### 1.6 `Padding` / `Margin` 的 `edges()` 语义

`ui/element.go:549`–`561` 的 `edges()` 返回 `[4]float32`，下标顺序是 **top, right, bottom, left**
（`PaddingX` 改 `pad[1]`/`pad[3]`，`PaddingY` 改 `pad[0]`/`pad[2]`，`ui/api_gen.go:3656`–`3672`）：

| 参数个数 | 含义 | 实测 `Padding(10,20,30,40)` |
|---|---|---|
| 1 | 四边 | — |
| 2 | 上下 / 左右 | — |
| 3 | 上 / 左右 / 下（**文档没写，源码支持**） | `Padding(10,20,30)` → 上 10、左右 20、下 30 ✓ |
| 4 | 上 / 右 / 下 / 左 | 上 10、右 20、下 30、左 40 ✓ |

实测方法：外层 Box 上色，内层子元素上另一种色，数像素边界。`Radius` 是另一套语义：
1 个值四角，4 个值是左上/右上/右下/左下（`ui/element.go:851`–`861`），**没有 2/3 个值**。

### 1.7 窗口尺寸、位置与"记住上次大小"

`mygo.NewWindow(mygo.WindowOptions{...})` 建窗口，`Content: ui.View(a.view)` 挂原生界面
（`internal/gui/view.go:41`–`56`、`:59`–`79`）。窗口内取尺寸用 `c.Size()`（`ui/api_gen.go:393`）。

**记住 bounds 的机制是 `WindowOptions.StateKey`**（不是自己存文件）：

```go
win := mygo.NewWindow(mygo.WindowOptions{
    Title: "leeway", Width: mainWidth, Height: mainHeight,
    MinWidth: 780, MinHeight: 500,
    TitleBarStyle:   mygo.TitleBarHidden,   // 自己画标题栏
    BackgroundColor: appearanceCanvas,      // 首帧前的底色
    StateKey:        "main",                // :51 主窗口
    Content:         ui.View(a.view),
})
```

`StateKey` 记住位置、大小、最大化和全屏状态——存的就是 `X/Y/Width/Height/Maximized/FullScreen`
（`window_state.go:24`–`31`）——放在 `PathUserData` 下的 `window-state.json`：窗口移动、缩放或换状态
稳定 300ms 后捕捉，关闭时和 App 退出时写（`window_state.go:12`–`20`、`paths.go:21`–`23`）。
同 key 的窗口下次启动取回，mygo 的文档说这些值会**覆盖**
`X/Y/Width/Height/UseContentSize/Maximized/FullScreen`（`window.go:152`–`159`）。
设置窗口用 `StateKey: "settings"`（`internal/gui/view.go:74`）。

### 1.8 排查版式：`Debug()` 和 inspector

- `Element.Debug()` 把元素和它内部所有元素的盒子、padding、margin 都描出来（`ui/api_gen.go:3256`–`3257`）。
- inspector（`ui/inspector*.go`）能看到每个元素的盒子和"为什么是这个盒子"。
- 元素自己的盒子：`Element.Bounds()` 返回**上一帧**的盒子（`ui/api_gen.go:3242`–`3247`）；
  本帧的盒子在绘制回调里拿：`Element.Draw(func(p *ui.Painter, r ui.Rect){...})`
  （`ui/api_gen.go:2783`）。本仓库的 `bar()`/点阵就是这么画的（`internal/gui/theme.go` 的 `bar()`）。

## 2. 渲染与测试

### 2.1 帧什么时候重建：不是每帧，是有事才建

`ui/doc.go`：view 是"一帧的界面"，mygo 在**输入之后、`Window.Update` 之后、`Invalidate` 之后、
以及有东西在动的时候**再调它。`docs/ui/transitions.md` 最后一句更直白：**Frames are drawn only while
something moves.** 另外窗口被遮住时，动画会被挂起（`ui/runtime.go:470`–`480`：`moving && occluded` → 不画）。

所以"无人值守时只能拿到开机那一帧"的正确解释不是"不重绘"，而是**没有输入就没有页面切换**：
`LEEWAY_GUI_CAPTURE` 走的是 `Window.CapturePage()`，它拿的是**最后一次呈现的帧**
（`ui/window.go:343`–`355`：`h.last` 重新栅格化）。本仓库的截图钩子在 `internal/gui/app.go:183`–`198`，
`Start` 后 1.5s 截一张就退出（`internal/gui/app.go:169`–`179`）。

要看别的页面，两条路：

1. 测试里 `ui.NewTester` 渲染（下面 2.2）；
2. 真窗口里人工点。

### 2.2 `ui.NewTester`：不建窗口跑完整界面

`ui.NewTester(view, width, height)`（`ui/headless.go:144`–`147`），文档见 `docs/ui/testing.md`。
本仓库的用法：`internal/gui/gui_test.go:372` 起，`ui.NewTester(a.view, mainWidth, mainHeight)`；
设置窗口用 `a.settingsView` + `settingsWidth/settingsHght`（`internal/gui/gui_test.go:585`）。

常用面（全部在 `ui/headless.go`）：

| 用途 | API |
|---|---|
| 找元素、断言文案 | `Find(s) (Rect, bool)`（:189，**精确**匹配 text 或 label）、`Texts()`（:200）、`HasText(s)`（:209，**子串**） |
| 点击/输入 | `Click(s)`（:219）、`ClickAt`、`ClickWith(ui.Shift, s)`、`Press`/`Release`/`Move`、`Scroll(x,y,dx,dy)`、`Key(mods,key)`、`TypeKey`、`Type`、`Compose`、`Command("copy")` |
| 菜单 | `RightClick(s)`、`Menu() []string`（:268）、`ChooseMenuItem("A","B")`（:285）、`CloseMenu` |
| 窗口与桌面 | `SetSize`、`SetScale`、`SetDark(true)`、`SetPreferences(ui.Preferences{ReduceMotion:true, TextScale:1})`、`SetTitleBar(ui.TitleBar{...})`、`SetFocused(bool)` |
| 帧与图像 | `Frame()`、`Image() *image.RGBA`；只画一张可以用 `ui.Render(view, w, h, scale)`（:100） |
| 无障碍/副作用 | `Announcements()`、`Focused(s)`、`Clipboard()`/`SetClipboard`、`OpenedURLs()`、`FailOpenURL(err)`、`Cursor()` |

三个容易踩的点：

- **`Find`/`Click` 只认可见元素。** 被滚动区裁掉的元素 `Find` 会返回一个 `W=0 H=0` 的 Rect
  （`ui/runtime.go:804`–`837`：不可见/惰性的元素不进 `rt.labels`）。实测：默认总览页里
  `Find("配置 key")` = `{X:912 Y:852 W:0 H:0}`（窗口 1040×700，在折叠线下面），`Click` 打在空气上；
  先 `a.navigate("provider:deepseek")` 再 `Find` 得到 `{X:871 Y:195 W:46.7 H:14.1}`，点击正常。
  **点不到先看 Rect 的 W/H 是不是 0。**
- **`Find`/`Click` 是精确匹配，`HasText` 是子串。** 同一个 label 出现多次时 `Find` 返回第一个，
  所以可点的 label 要唯一、可读（见 3.1）。
- **同一父节点下两个元素给了同一个 `Key` 会 panic**（`ui/frame.go:140`–`143` 的说明，
  `docs/ui/testing.md`）。测试里遇到 panic 先查 `Key`。
- **`Find` 返回的是窗口绝对坐标，不是"某个容器内的坐标"。** 量"页面到底了没有"时最容易错：
  标题栏下面那一行的高度是 `winH - titleBarHeight(c)`，**那是高度，不是 y**。滚动区的视口在窗口
  坐标里是 `y ∈ [titleBarMin, winH]`，下沿就是 `winH`。曾经 `TestTheEndOfALongPageIsReachable`
  写成 `viewport := mainHeight - titleBarMin`（= 646）再拿去当 y 用，于是它要求的留白比设计多
  2 DIP，**恰好以 0 DIP 余量通过**；字号一变大就翻红，看着像"滚动到底还差 32 DIP"，其实是判据
  的坐标系错了。实测：`Find("leeway")` ≈ `y=18`、`Find("总览")` ≈ `y=96`（都是窗口坐标），
  而滚动到底时最后一张卡的下边缘画在 `y=660`，下面 40 DIP 全是背景——`Padding` 的下边距一直
  是好的。判据改成 `mainHeight - pageBottomPad`（`view.go` 里同一个常量）之后余量 12 DIP，
  并且仍能抓住真正的回归（把 `Row.Height(winH - titleBarHeight)` 写回 `winH`、或去掉
  `FillHeight()`、或让页面列自己超出盒子，都会重新翻红）。

### 2.3 菜单项不在元素树里，但**能测**——用 `Tester.Menu()`

菜单项是 `platform.MenuItem`，不是 `Element`：`Element.Menu(build)` / `MenuButton` 在菜单打开时
把 `build` 收进一个 `platform.Menu`，交给宿主弹原生浮层（`ui/api_gen.go:2592`–`2632`、`ui/menu.go:399`–`436`）。
所以 `Texts()`/`HasText()`/`Find()` **看不到** `m.Item("…")` 的文案——这一点是对的。

但"点开也看不到"是**错的**：headless 宿主把弹出的菜单记下来了，`Tester.Menu()` 返回它的标签，
`Tester.ChooseMenuItem(path...)` 还能选中它（`ui/headless.go:120`–`122`、`:268`–`:325`，
文档 `docs/ui/testing.md` 的 **Menus** 一条）。实测（把探针作为 overlay 测试挂进本仓库跑的）：

```
Find("配置 key") = {X:871.3 Y:194.9 W:46.7 H:14.1} ok=true
menu: []string{"粘贴 API key…"}
after choosing "粘贴 API key…", texts: [… 粘贴 API key  DeepSeek API key 显示明文 … 取消 保存]
```

以及标题栏那个刷新周期菜单：`Click("手动刷新")` → `Menu()` =
`["手动刷新","每 30 秒","每分钟","每 5 分钟","每 15 分钟"]`。

所以菜单的测法有两种，按成本选：

```go
// 1) 端到端：点开菜单，看它列了什么，选一项，再看界面变成了什么。
tester := ui.NewTester(a.view, mainWidth, mainHeight)
if err := tester.Click("配置 key"); err != nil { t.Fatal(err) }
if got := tester.Menu(); !slices.Equal(got, []string{"粘贴 API key…"}) { t.Errorf("menu = %v", got) }
if err := tester.ChooseMenuItem("粘贴 API key…"); err != nil { t.Fatal(err) }
if !tester.HasText("DeepSeek API key") { t.Errorf("the sheet did not open; texts=%v", tester.Texts()) }

// 2) 只测"菜单该有什么"：把内容做成纯函数，单测纯函数。
//    internal/gui/view.go:521 loginMethods / :553 loginButtonLabel
//    internal/gui/gui_test.go:1282 TestProviderActionsMatchWhatTheProviderSupports
```

第 2 种仍然是好习惯（菜单内容由 `provider.AuthSpec` 派生，纯函数最好测），但它不是"因为菜单测不到"
才这么做的。

### 2.4 版式验证：先量，再数像素

**mygo 有公开的文字测量 API，别一上来就数像素。**

```go
// ui/richtext.go:226-232：宽度 0 = 不换行，返回 max-content 尺寸。
w, h := c.MeasureText(0, ui.Span{Text: label, Size: sizeBody, Weight: 600})
if w > available { /* 放不下，换短文案或加 Tooltip */ }

// 想知道换成某个宽度会折几行：
w2, h2 := c.MeasureText(60, ui.Span{Text: "hello world", Size: 13})
// 实测：no-wrap 65.87 x 15.31；wrap@60 → 33.34 x 30.62（两行）
```

- 签名：`Context.MeasureText(width float32, spans ...Span) (w, h float32)`（`ui/richtext.go:226`–`232`），
  绘制期还有 `Painter.MeasureText`（`ui/richtext.go:219`–`224`）；`Span` 的字段见 `ui/richtext.go:15`–`33`，
  零值的 `Font`/`Size` 用主题的字体和字号（`ui/richtext.go:173`–`175`）；`Color` 只影响绘制，
  测量不带它（`spanStyle` 只搬 Family/Size/Weight/Italic/LetterSpacing/Features，`ui/richtext.go:156`–`158`）。
- 元素盒子用 `Element.Bounds()`（上一帧，`ui/api_gen.go:3242`）或 `Draw` 回调里的 `Rect`
  （本帧，`ui/api_gen.go:2783`）。
- **没有公开的"这段文字被截断了吗"信号。** `text.Layout.Truncated` 只存在于内部包
  （`internal/text/layout.go:228`–`229`、`:605`），`ui` 没有导出。想知道会不会被截，
  只能自己 `MeasureText` 跟可用宽度比，或者干脆无条件挂 `Tooltip`。

什么时候才需要数像素：**整图比对**（动画有没有生效、深浅色有没有差、某块区域有没有画出来）。
本仓库现成的工具：

- `internal/gui/gui_test.go:1276`–`1288` 的 `meanDifference(x, y image.Image) float64`：
  逐像素三通道平均差，`internal/gui/gui_test.go:1245`–`1272` 用它断言
  "150ms 时的入场帧 ≠ 静止帧"和"ReduceMotion 下 ≈ 静止帧"。
- 快照：`internal/gui/gui_test.go:144`–`163` 的 `snapshot()`，只在设了 `LEEWAY_GUI_SNAPSHOT` 时写 PNG：

  ```sh
  LEEWAY_GUI_SNAPSHOT=/tmp/shots go test ./internal/gui/   # 32 张 1040×700 的 PNG，人眼过版式
  ```

- 拼图看整体：`resources/screens.py`（Pillow，把 `build/screenshots` 里的图拼成一张板）。
  它不是断言，只是给人看的。
- 单独量一条线/一个色块：`tester.Image()` 后按坐标取样（本文 1.2/1.3/1.4 的数字都是这么来的）。

**快照可复现性有个尾巴**：本仓库把"现在"钉在 `pinned`（`internal/gui/gui_test.go:48`，
`a.clock` 见 `internal/gui/app.go:58`–`60`），所以同一份代码两次跑出的快照基本一致。
实测两张不是逐字节可复现，而且它们坏得不一样：
- `provider-many-burn.png` 两次运行差 13745 个像素，**最大通道差 2/255**，集中在底部一条还在跑
  颜色过渡的带（y 628..698）——"颜色过渡会漂一个最低位"。
- 同一张图在**另一对运行**里（冻结帧 vs 重跑）差到 **最大通道差 13/255、11739 个像素**，同一段
  y 626–699：`snapshotStill` 抓的是 toast 淡入的中途帧，相位不同就差得多。
- `overview-probing*.png` 差 288/289 个像素、最大通道差 94（转圈的相位）。
所以：**别用逐字节相等断言快照**，用 `meanDifference` 这种带阈值的比较；也别把"最大 2/255"
当成上限——它是其中一对运行的结果。

### 2.5 纯 Go、无 cgo、不需要显示器

`go test ./internal/gui/` 直接跑完整渲染测试；`CGO_ENABLED=0 go test ./internal/gui/` 一样通过
（实测，两次都 `ok leeway/internal/gui`）。`ui` 包里没有 cgo，macOS 的系统调用走
`github.com/ebitengine/purego`（`go.mod` 的间接依赖）。

## 3. 组件与风格

### 3.1 builder：`ui.Context` 决定"新元素挂给谁"

每个构造函数都要 `c`，元素挂到**当前父节点**（`ui/context.go:191`–`221` 的 `newElement`：
`e.id = mix(p.id, uint64(p.nchild)+uint64(k)<<56)`，然后 `p.add(e)`）；
`Children(func(){...})` 换父节点再换回来（`ui/element.go:476`–`489`）；`ui.Spacer(c)` 就是
`Box(c).Grow(1)`（`ui/api_gen.go:1871`–`1872`）。**元素的身份由"父节点 + 第几个孩子 + kind"决定**，
所以跨帧的状态（焦点、滚动、正在编辑的文本、动画）跟着位置走；要让它跟着数据走就 `Key(k)`
（`ui/frame.go:140`–`143`，注意带输入处理的控件本身不能加 `Key`，要加在外面的容器上）。

```go
ui.Column(c).FillWidth().Gap(22).Children(func() {   // 这之后创建的元素都挂在 Column 下
    ui.Text(c, "第一块")
    card(c, p, func() {                              // card 内部再 Children，挂到卡片里
        ui.Text(c, "卡片内容")
    })
    ui.Spacer(c)                                     // 占掉剩余空间
})
```

`internal/gui/` 里所有绘制入口都是这个形状：`App.view(c)`、`App.settingsView(c)`、
各页面的 `func (a *App) xxxPage(c *ui.Context, p palette)`。

**可测性也是设计约束**：纯图标按钮一定要给 `Label`，否则测试点不到——
窗口里每个纯图标按钮都在建的时候自己给了 `Label`（例如 `internal/gui/view.go` 的 `ui.Button(c, "").Label("刷新")`）——MyGo 的 `Button` 不强制，漏了就只有测试点不到。

### 3.2 本项目 theme 约定（`internal/gui/theme.go` 一处说了算）

- 卡片是三层（`internal/gui/theme.go` 的 `card()`/`shell()`）：两者是同一块板子，差别只在吃不吃悬停——外层托盘
  `Padding(3).Radius(19).Background(p.bezel).Border(1, p.border)`，内层
  `Padding(18).Gap(14).Radius(16).Background(p.card)`，悬停时两者各换一档色（`p.bezelHi`/`p.cardHi`），
  整体套 `panelMotion`。页面只调 `card(c, p, fn)`。
- 细分隔线：`hairline()` = `ui.Box(c).FillWidth().Height(1).Background(p.border)`
  （`internal/gui/theme.go` 的 `hairline()`）；列间竖线用容器自带的 `Dividers`（见 1.2）。
- 小圆点：`dot()` = `ui.Box(c).Size(8,8).Radius(4).Background(color)`（`internal/gui/theme.go` 的 `dot()`）。
- 颜色全走 `palette`（`internal/gui/theme.go` 的 `paletteOf()`），深浅各一套，`t.Dark` 判断；
  构造用 `ui.RGBA`（`ui/color.go:29`）、`ui.Hex`（`ui/color.go:33`）、`ui.Oklch`（`ui/widecolor.go:18`），
  `Mix`/`Alpha` 派生。**力度是量出来的**：`border` 浅色 7% 黑 / 深色 10% 白，
  `divider`（列竖线）浅色 **18% 黑** / 深色 **26% 白**——比 `border` 重一档才看得出来
  （`internal/gui/theme.go` 的 `paletteOf()`（`border`/`divider` 两档））。
- 主题写回：`c.Theme()` 拿当前主题，改一份再 `c.SetTheme(&t)`
  （`internal/gui/theme.go` 的 `paletteOf()`；mygo 侧 `ui/api_gen.go:403`–`411`）。
  深浅色由应用级开关决定：`mygo.Theme.SetSource(mygo.ThemeLight/Dark/System)`
  （`internal/gui/view.go:82`–`91`；mygo 侧 `modules.go:224`–`226`、`:249`）。
- 字号/字重集中在 `internal/gui/theme.go` 的字号常量（`sizeDisplay`…`sizeChrome`）（六个尺寸），数字一律
  `FontFeatures("tnum")` 等宽。
- 无障碍：标题用 `Role(ui.RoleHeading).Level(1|2)`（`internal/gui/theme.go` 的 `title()`/`section()`），
  装饰性元素 `Role(ui.RoleNone)`（`internal/gui/theme.go` 的装饰性图标调用点）。

### 3.3 图标：两种来源，两套 API（别混）

**（a）界面图标 = 内联 Lucide SVG**，跟文字颜色走：

```go
// internal/gui/icons.go:10-12
func icon(svg string) *ui.SVG { return ui.MustParseSVG([]byte(svg)) }

var iconRefresh = icon(`<svg … viewBox="0 0 24 24" stroke="currentColor">…</svg>`)

// 画：
ui.Icon(c, iconRefresh).FontSize(15).TextColor(p.t.TextMuted)   // 见 internal/gui/view.go 的刷新按钮
```

`ui.Icon(c, s *SVG)` 收的是 `*ui.SVG`（`ui/api_gen.go:1069`–`1076`），`FontSize` 决定它多大
（`docs/ui/icon.md`）。

**（b）provider 图标 = 各家发布的 PNG，`//go:embed` 进二进制**：

```go
// internal/gui/icons.go:56-66
//go:embed icons/*.png
var iconFiles embed.FS

var marks = map[string]*ui.Bitmap{}

func providerMark(id string) *ui.Bitmap {
    if b, ok := marks[id]; ok { return b }
    var b *ui.Bitmap
    if data, err := iconFiles.ReadFile("icons/" + id + ".png"); err == nil {
        b, _ = ui.DecodeBitmap(data)          // ui/bitmap.go:38
    }
    marks[id] = b                             // 解码几毫秒，解一次就够
    return b
}

// 画（见 internal/gui/theme.go 的 `icon()`）：
e.Children(func() { ui.Image(c, b).Size(size, size) })   // ui/widgets.go:534，收 *Bitmap 或 *SVG
```

来源和加工方式记在 `internal/gui/icons/SOURCES.md`（七个 provider 各自的官方地址，
`resources/provider-icons.py` 统一裁成 256px 圆角方块）。**provider 图标必须用官方素材**
（用户明确要求），没有素材时退回"专属色相 + 首字母"，并有
`TestEveryProviderHasItsOwnIcon`（`internal/gui/gui_test.go:879`）盯着不让退路悄悄发生（退路本身在 `internal/gui/theme.go` 的 `providerIcon()`）。

### 3.4 动效：过渡用 `ElementTransition`，入场用 `AnimationFrame`

- 页面/卡片过渡：`Element.Transition(ui.ElementTransition{...})`（`ui/transition.go:17`–`71`），
  字段 `Duration`（0 = 200ms）、`Ease`（nil = `ui.EaseOut`）、`Position/Size/Colors`、
  `Enter`/`Exit *ui.Motion`（`X/Y/Opacity/Collapse`）。本仓库三个预设：
  `pageMotion`（只淡入）、`stateMotion`（只收色，140ms，指针下的状态）、
  `panelMotion`（收色 + 上浮 8px 落位，240ms）。**一个元素只能挂一条过渡**，后挂的覆盖先挂的
  （`ui/transition.go:255`–`258`：`r.t = u.t // the element's last Transition counts`），
  所以卡片是三层：最外层只落位，bezel 和表面各挂 `stateMotion` 收色（`internal/gui/theme.go`）。
  `Opacity` 会沿着绘制树相乘（`ui/paint.go:120`–`128` 每层 `p.opacity *= e.opacity`），
  所以挂在最外层也能让整张卡淡入。
  自定义缓动就是一个 `ui.Easing`（`func(t float32) float32`，`ui/widgets.go:114`）：
  `easeGlide` = `1 - (1-t)^3.4`（`internal/gui/theme.go` 的 `easeGlide`）。
- **自己设 `FontSize` 就等于关掉了系统的文字大小**：`ui.Theme.FontSize` 会被 `TextScale` 乘，
  但 `Element.FontSize(v)` 是绝对值。要么别自己设，要么把尺寸做成变量、每帧按
  `c.Preferences().TextScale` 重算一次（本仓库的 `scaleType()`）；后者不用改 40 个调用点。
- **`MaxLines(1)` 不阻止换行，`SingleLine()` 才阻止**：宽度不够时 `MaxLines` 只是限制行数，
  文字会在极窄的格子里**一字一行**竖着排（本仓库的合并行在 780px 下就是这样塌的）。要「截断成
  省略号、绝不堆叠」就用 `SingleLine()`；宽的那部分再包一层 `Grow(1)+Clip()` 让它是唯一让步的。
- **命中可以交给布局，画笔只负责画**：`Element.Hovered()` 是一行等宽隐形格子就能做的事——
  一行 `Grow(1).FillHeight()` 的空盒子，谁 `Hovered()` 谁就把下标写进闭包变量；图用
  `DrawOver` 画（它在该元素的子元素之后绘制，拿到的 rect 就是整行），闭包在**绘制时**才读那个
  变量，所以构建顺序不影响结果。比自己做命中测试省事得多（uix 的 linechart 同法）。
  注意：隐形格子会参与命中，压在可点的卡片上时要验证卡片本身还点得开。
- **谁负责写这个时间字段**：本仓库曾有一套入场动画，它读一个 `at` 字段；主窗口从来没人写这个
  字段，所以动画静默失效（测试里手动写了，看起来是好的）。这类「谁负责写」的问题只能回到生产
  路径上看。教训有两层：一是**测试自己填状态会让死代码看起来是活的**；二是修好之后要重新判断
  这个效果该不该存在——补上锚点会让第一帧整页透明，而监控工具的第一帧必须是答案，于是整套
  删掉比修好更对。
- 值动画：`Element.Animate(key, target, d) float32` / `AnimateWith(key, target, d, ease)`
  （`ui/api_gen.go` 的 `Element.Animate`）——首帧直接返回 target（所以第一次显示不会假装从 0 数上来），
  自己会保持帧率。本仓库 `numberMotion = 520ms`（`internal/gui/theme.go`），数字和它下面的条共用。
- 手写动画（在**帧内**读时间、按进度设样式，并调 `c.AnimationFrame()` 让引擎继续出帧，
  `ui/api_gen.go:247`）：本仓库**现在没有这种代码**，入场编排已经整段删掉（见下条）。要写的话
  两个坑还在：**被包的元素如果自带过渡，它进入时会写自己的 opacity，把外层设的透明度盖掉**
  （`panelMotion` 就会），所以要包一层不带过渡的 holder；`Margin(0,0,x,0)` 是**下**边距，只把后
  一块往下推，不会把本块抬起来——要抬本块用上边距。
- **入场编排：本仓库删掉了，别再照着老配方加回来。** 曾经有一套「每块晚 55ms 起步、上浮 10 DIP
  淡入」（`internal/gui/arrival.go`，已删除）：第一帧整页透明，监控工具打开先什么都不给看，
  评审规则把「监控工具上的错落入场」直接列为 instant-fail。删掉的理由和两个坑（每块的时间戳字段
  没人写 = 静默死掉的动画；卡片过渡覆盖外层 opacity）记在 `docs/GUI.md`「没有入场动画」一节，
  能钉的只有「第一帧已经有数字」（`TestOverviewRendersEveryProvider`）；动效本身写不出会失败的断言。
- `plugins/glass`（`plugins/glass/glass.go:29` 的 `type Glass`）**不是动画**，是背景模糊材质：`ui.Column(c).Material(glass.Glass{})`
  （`internal/gui/view.go:143`、`:198`），给标题栏和侧栏做"玻璃 chrome"。

## 4. 构建与分发

### 4.1 `mygo build`

```sh
go run github.com/egoist/mygo/cmd/mygo@v0.3.4 build        # 平台默认 = 本机 darwin/arm64
go run github.com/egoist/mygo/cmd/mygo@v0.3.4 build -skip-dmg
```

配置在 `mygo.json`（`name`/`identifier`/`version`/`main`/`out`/`copyright`；
`out` 默认 `dist`，本项目写 `build`）。产物目录是 **`<out>/<goos>-<goarch>/`**：

```
build/darwin-arm64/leeway.app
build/darwin-arm64/leeway 0.1.0.dmg        # dmgFileName = "<Name> <Version>.dmg"
```

依据：`cmd/mygo/build.go:22`–`52`（命令说明）、`:130`–`141`（`<out>/<goos>-<goarch>` 与 staging 后替换）、
`:213`–`:236`（写 bundle、签名、再建 dmg）、`cmd/mygo/dmg.go:17`–`19`（dmg 文件名）、
`cmd/mygo/config.go:19`–`46`（字段与默认值）。

**版本号以 bundle 的 Info.plist 为准**：`mygo.json` 的 `version` 被写进 `CFBundleVersion` 和
`CFBundleShortVersionString`（`cmd/mygo/bundle.go:79`–`83`），Finder/关于本机读的是它。
`internal/gui/view.go:19` 的 `Version` 常量是进程内自报的版本（`cmd/leeway-gui/main.go` 的
`mygo.App.SetVersion`），两边要一起改。实测 `/Applications/leeway.app` 的 plist：
`CFBundleIdentifier = dev.helixliu.leeway`、`CFBundleShortVersionString = 0.1.0`。

### 4.2 装完删掉开发目录里的 `.app`，否则 Spotlight 有两份

把 `build/darwin-arm64/leeway.app` 拖进 `/Applications` 后，开发目录那份**也要删掉**：
Spotlight 按 bundle id 索引，两份都在时"打开方式/聚焦搜索"会列两条。

**在子目录里放 `.metadata_never_index` / `.noindex` 没用**（实测，见下表）。
本仓库当前的状态正好符合这条纪律：`build/darwin-arm64/` 里只剩 `.dmg`，`.app` 已经删了；
`mdfind "kMDItemCFBundleIdentifier == 'dev.helixliu.leeway'"` 只返回 `/Applications/leeway.app`。
要么删掉开发目录那份，要么把 `build/` 加进 Spotlight 隐私（系统设置 → Spotlight → 隐私）。
`build/` 本来就在 `.gitignore` 里（`.gitignore:8`）。

实测证据（在 `$HOME` 下建两个非隐藏目录，各放一个假的 `.app`，`guarded` 里放那两个标记，
等 40 秒后按 bundle id 查；探针跑完已删除）：

```
plain  （无标记）        → ~/mygo-spotlight-probe/plain/ctrlprobe111.app      ← 被索引
guarded（.metadata_never_index + .noindex）→ …/guarded/guardprobe222.app                 ← 一样被索引
```

对照组的 `plain` 能被找到，说明索引本身工作；`guarded` 也被找到，说明标记在子目录无效
（`.metadata_never_index` 只在卷根生效）。

### 4.3 校验手段

```sh
codesign --verify --deep --strict --verbose=1 /Applications/leeway.app
hdiutil verify "build/darwin-arm64/leeway 0.1.0.dmg"
```

实测两条都通过：`valid on disk` / `satisfies its Designated Requirement`，
`hdiutil: verify: checksum of "…dmg" is VALID`。mygo 自己也会在 build 里签名
（`cmd/mygo/bundle.go:189`–`222`，`cmd/mygo/dmg.go:95`–`100`）；
要发到别的机器上需要 `macos.signingIdentity` + `macos.notarize`（`cmd/mygo/config.go:121`–`146`）。

### 4.4 无人值守的真窗口截图

```sh
LEEWAY_GUI_CAPTURE=/tmp/window.png open build/darwin-arm64/leeway.app
```

注意两点：

- **需要 `.app` 存在**。按 4.2 的纪律删掉之后，先重新 build 再跑这条命令。
- **不需要屏幕录制权限。** 它走 `Window.CapturePage()`，对 `Content` 窗口是**自己把最后一帧重新
  栅格化成 PNG**（`window.go:1085`–`1104`、`ui/window.go:343`–`355`、`content.go:184`–`209`）。
  需要屏幕录制权限的是系统的 `screencapture`（本机未授权时报
  `could not create image from display`），跟这条路无关。

## 5. 坑清单

现象 → 原因 → 正确写法 → 证据。带 ✅ 的是原始记录里核实无误的，带 ✏️ 的是核实后修正过说法的。

| # | 现象 | 原因 | 正确写法 | 证据 |
|---|---|---|---|---|
| 1 ✏️ | 标题栏下面的滚动区比看得见的区域高出正好一个标题栏，页面最后一截滚不到；改成显式高度才正常 | **不是"按窗口解析"，是按父容器的 content box 解析**（实测：300 宽的父容器加 `Padding(20)`，子元素 `FillWidth()` 只有 260 宽）。所以父容器的 content box 比可视区高多少，视口就高多少；父容器高度是 auto（或 `Scroll` 内部，内容高度 `inf`）时百分比解析不出来，静默退化成 auto，滚动容器长到内容高度、根本滚不动 | 要"剩下的高度"就自己算：`_, winH := c.Size()` + `Row.Height(winH - titleBarHeight(c))`；在 Column 里也可以只用 `Grow(1)`（flex 的 shrink 会收回来） | mygo `ui/element.go:56`–`66`、`ui/layout.go:61`–`66`、`ui/layout.go:455`–`457`、`ui/layout.go:513`–`565`、`docs/ui/layout.md:33`–`36`；本仓库 `internal/gui/view.go:128`–`156`、`:173`–`182`；实测 300 ✓ / 338.7 ✗ / 954 ✗ |
| 2 ✅ | `AlignSelf(ui.Stretch)` 能让 1px 的 Box 跟整行一样高 | `selfAlign` 先看元素自己的 `self`，再退回父容器的 `align`；Stretch 沿交叉轴拉伸 | `ui.Box(c).Width(1).Background(p.divider).AlignSelf(ui.Stretch)` | mygo `ui/layout.go:810`–`820`；本仓库 `internal/gui/page_matrix.go` 的 `Dividers` 用同一手法；实测 0..119 vs 什么都没有 |
| 3 ✏️ | 空的预测行没有留下多余高度 | Stretch **不是**不生效：它填的是**这一行的行高**（最高兄弟的高度）；行里没有任何元素有内在高度时行高才是 0。另外 Stretch 只管交叉轴——Column 里管宽度，不管高度 | 行高靠兄弟撑；要固定行高就写 `Height`；Column 里别指望 Stretch 变高 | mygo `ui/layout.go:627`–`644`（`ln.cross` 取行内最大）、`ui/layout.go:637`–`643`（有确定高度就不拉伸）；实测 Row 兄弟 60 高 → 线 0..59 |
| 4 ✅ | 往 Row 里插 1px 分隔线后每列变窄约 10px | `Gap` 加在**每一对相邻子元素**之间，插一个线多出两个 gap | 能接受就接受；想省宽度用容器自带的 `Dividers(width, color)`，它画在已有 gap 中间、不占宽度 | mygo `ui/api_gen.go:3070`、`ui/api_gen.go:2680`–`2686`、`ui/layout.go:516`–`522`、`docs/ui/layout.md:23`–`25`；实测 `[0 138 276]` vs `[0 143 286]` |
| 5 ✏️ | 长文本会把列撑开 | 真正的原因是 Row 子元素的**自动最小宽度 = min-content**（`intrinsic(c, false)`），它让 flex 分配不均、最后一列被挤成窄条 | 每一列都写 `Grow(1).MinWidth(0)`，再用 `SingleLine()`/`MaxLines(n)` 把文字收进去 | mygo `ui/layout.go:554`–`564`；本仓库 `internal/gui/page_matrix.go:65`、`:78`；实测 183.3/116.7/0 vs 100/100/100（最后一列被顶出窗口）|
| 6 ✅ | 两个 Text 挨着渲染成 `5-hour已用` | Row 没有 `Gap` | 给容器 `Gap(6)`；**别用空格拼文案** | 本仓库 `internal/gui/page_matrix.go:92`、`:100`；实测 41.58 = 41.58 |
| 7 ✅ | `Padding`/`Margin` 的多值语义 | `edges()` 的下标是 top, right, bottom, left | 1 = 四边；2 = 上下/左右；3 = 上/左右/下（文档没写但源码支持）；4 = 上/右/下/左。`Radius` 是另一套：1 或 4 个值 | mygo `ui/element.go:549`–`581`、`ui/element.go:851`–`861`；实测 10/20/30/40 与 10/20/30 |
| 8 ✏️ | 窗口初始宽高 / 记住 bounds | 待核实的机制已确认：`mygo.WindowOptions.StateKey` | 建窗口时给 `StateKey`（主窗口 `"main"`、设置 `"settings"`），mygo 存 `window-state.json`（`PathUserData`），下次启动取回并覆盖 X/Y/W/H | mygo `window.go:152`–`159`、`window_state.go:12`–`20`、`paths.go:21`–`23`；本仓库 `internal/gui/view.go:41`–`56`、`:59`–`79` |
| 9 ✏️ | 无人值守截图只能拿到开机那一帧 | 准确说法是**帧只在有事时重建**（输入、`Update`、`Invalidate`、有东西在动），被遮住时动画还会挂起；没有输入就没有页面切换 | 要看别的页面就在测试里 `ui.NewTester`，或人工点；`LEEWAY_GUI_CAPTURE` 只够自检首屏 | mygo `ui/doc.go`、`docs/ui/transitions.md`（末句）、`ui/runtime.go:470`–`480`；本仓库 `internal/gui/app.go:183`–`198` |
| 10 ✅ | 测试入口 | — | `ui.NewTester(view, w, h)`；另有 `Find`/`Menu`/`ChooseMenuItem`/`SetTitleBar`/`SetPreferences`/`Announcements`/`Scroll`/`Key`/`Type` 等 | mygo `ui/headless.go`、`docs/ui/testing.md`；本仓库 `internal/gui/gui_test.go:372`、`:570` |
| 11 ✏️ | 菜单测不到 | **菜单项确实不在元素树里**（`Texts()`/`HasText()` 看不到），但 headless 宿主记下了弹出的菜单，`Tester.Menu()` 能列出、`ChooseMenuItem()` 能选。原始那次失败的真实原因是 `Find("配置 key")` 命中的按钮被滚动区裁掉了（`Rect` 的 `W=H=0`），点了个空气 | 端到端用 `Click` → `Menu()` → `ChooseMenuItem(path…)`；"菜单该有什么"仍然适合抽成纯函数单测 | mygo `ui/api_gen.go:2592`–`2632`、`:315`–`:352`、`ui/headless.go:120`–`122`、`:268`–`:325`、`ui/runtime.go:804`–`837`、`docs/ui/testing.md`（Menus）；本仓库 `internal/gui/view.go:505`–`513`、`:521`–`:549`、`internal/gui/gui_test.go:1297`；实测 `{W:0 H:0}` 与 `menu: ["粘贴 API key…"]` |
| 12 ✅ | `Click` 匹配的是控件 label | `Find`/`Click` 精确匹配元素的 text 或 `Label`，重复时取第一个 | 可点的控件都要唯一、可读的 label：`MenuButton(c, loginButtonLabel(id), …)`、`ButtonBase(c).Label("账号操作")` | mygo `ui/headless.go:227`–`264`、`ui/runtime.go:824`–`837`、`docs/ui/testing.md`；本仓库 `internal/gui/view.go:505`、`internal/gui/theme.go` 的 `chrome()` |
| 13 ✏️ | 没有文字测量 API，只能渲染 PNG 数像素 | **有公开 API**：`Context.MeasureText(width, spans...)`、`Painter.MeasureText`；元素盒子有 `Bounds()`（上一帧）和 `Draw` 回调的 `Rect`（本帧） | 先量：`w, _ := c.MeasureText(0, ui.Span{Text: s, Size: sizeBody})`，跟可用宽度比；数像素只留给整图比对（`meanDifference`） | mygo `ui/richtext.go:219`–`232`、`ui/richtext.go:15`–`33`、`ui/api_gen.go:2783`、`ui/api_gen.go:3242`–`3247`；本仓库 `internal/gui/gui_test.go:1276`–`1288`、`resources/screens.py` |
| 14 ✏️ | `MaxLines(n)`/`SingleLine()` 的行为 | mygo 行为本身确认：`MaxLines(n)` 最多 n 行、超出加省略号；`SingleLine()` = `single + maxLines(1)`；`Ellipsis(s)` 换省略号；`NoWrap()` 不折行也不加省略号。**"渲染时把完整文本放进提示里"不是 mygo 干的**，是本仓库自己挂的 `Tooltip`。而且 mygo **没有**公开"是否被截断"的信号 | 窄列里主动写短；需要悬停看全文就自己 `.Tooltip(全文)`；判断放不放得下用 `MeasureText` | mygo `ui/api_gen.go:3041`–`3055`、`internal/text/layout.go:67`–`70`、`:228`–`229`、`:787`–`798`；本仓库 `internal/gui/theme.go` 的 `keyValue()`、`internal/gui/settingsview.go:262`–`264`；实测 ink 行数：不限 62 / `SingleLine()` 13 / `MaxLines(1)` 13 / `MaxLines(2)` 26（正好 n×13）|
| 15 ✅ | `ui.Context` 是 builder | `newElement` 挂到 `c.parent`；`Children` 换父再换回；元素身份 = 父 + 序号 + kind | 每个构造函数都传 `c`；`Children(func(){…})` 分块；`ui.Spacer(c)` 占剩余空间；跨帧身份要跟着数据就用 `Key` | mygo `ui/context.go:191`–`221`、`ui/element.go:476`–`499`、`ui/api_gen.go:1871`–`1872` |
| 16 ✅ | 本项目设计系统 | 都在 `internal/gui/theme.go` | `card()`=`shell()`（托盘 + 内层卡）、`hairline()`、`dot()`、`chromeRule()`；列间竖线用 `Dividers`；颜色全走 `palette`（`ui.RGBA`/`Hex`/`Oklch`），`t.Dark` 分深浅 | 本仓库 `internal/gui/theme.go` 的 `card()`/`shell()`/`hairline()`/`dot()`/`paletteOf()` |
| 17 ✏️ | 图标用官方素材 + `//go:embed` | 来源对，**API 写错了**：`ui.Icon(c, …)` 收的是 `*ui.SVG`（内联 Lucide 路径），不是 PNG 字节；嵌入的 PNG 走 `ui.DecodeBitmap` → `*ui.Bitmap` → `ui.Image(c, b)` | 界面图标：`ui.MustParseSVG` + `ui.Icon(c, svg).FontSize(n).TextColor(c)`；provider 图标：`//go:embed icons/*.png` + `ui.DecodeBitmap` + `ui.Image(c, bmp).Size(s,s)` | mygo `ui/svg.go:51`–`88`、`ui/bitmap.go:38`–`50`、`ui/api_gen.go:1080`–`1092`；本仓库 `internal/gui/icons.go:10`–`12`、`:56`–`66`、`internal/gui/theme.go` 的 `icon()`、`internal/gui/icons/SOURCES.md` |
| 18 ✅ | 列分隔线要比 hairline 重一档 | 实测力度：`border` 浅 7% 黑 / 深 10% 白，`divider` 浅 18% 黑 / 深 26% 白 | 竖线用 `p.divider`，卡片内细分隔用 `p.border` | 本仓库 `internal/gui/theme.go` 的 `paletteOf()`（`border`/`divider` 两档） |
| 19b ✏️ | `ui.Toast` 的淡入不受 `ReduceMotion` 管 | 实测：`SetPreferences(ReduceMotion:true)` 下数据变化帧与静止帧逐像素相同（0 差异），但 toast 的淡入照样在跑（开/关 RM 都是 181/10737 px 差异）——它是库自己起的过渡，窗口关不掉 | 要严格「ReduceMotion 下没有动」就得不用 toast 的入场，或等库把偏好接进去；本窗口的选择是留着 toast（它是失败的唯一反馈通道），并把这条写在这里 | mygo `ui/toast.go`；实测 `overview-stale.png` 与 RM 帧对比 |
| 19 ✏️ | 动效与主题 API 的写法 | 过渡是 `ui.ElementTransition`，**一个元素一条过渡，后挂的赢**；值动画 `Animate` 首帧直接返回目标；**`mygo/plugins/glass` 与淡入无关**，它是背景模糊材质。手写入场需要帧内读时间 + `c.AnimationFrame()` 续帧 + 尊重 `ReduceMotion`——**本仓库已整段删除**（第一帧必须是答案） | 被包元素自带过渡会覆盖外层 opacity；`Margin` 用上边距；过渡预设放一处（`pageMotion`/`stateMotion`/`panelMotion`）；颜色是**构建期**读的，`SetTheme` 不是作用域（见 §3 末） | mygo `ui/api_gen.go:247`、`ui/transition.go:17`–`71`、`ui/api_gen.go` 的 `Element.Animate` 与 `Panel` 文档、`ui/context.go` 的 `SetTheme`、`plugins/glass/glass.go:29`；本仓库 `internal/gui/theme.go` 的 `panelMotion`/`pageMotion`/`stateMotion`、`internal/gui/view.go` 的页面切换 |
| 20 ✅ | `mygo build` 出 `.app` + `.dmg` | 产物在 `<out>/<goos>-<goarch>/`，dmg 名是 `<Name> <Version>.dmg` | `go run github.com/egoist/mygo/cmd/mygo@v0.3.4 build`；配置 `out`/`name`/`version` | mygo `cmd/mygo/build.go:22`–`52`、`:130`–`141`、`:213`–`:236`、`cmd/mygo/dmg.go:17`–`19`、`cmd/mygo/config.go:19`–`46`；本仓库 `mygo.json` |
| 21 ✅ | 装完要删掉开发目录里的 `.app` | Spotlight 按 bundle id 索引；`.metadata_never_index`/`.noindex` 放在**子目录**无效（只在卷根生效） | 删掉 `build/darwin-arm64/leeway.app`，或把 `build/` 加进 Spotlight 隐私 | 实测：有标记的目录照样被 `mdfind` 找到（对照组也能找到）；本仓库 `build/darwin-arm64/` 只剩 dmg、`.gitignore:8` |
| 22 ✅ | 校验手段 | — | `codesign --verify --deep --strict <app>`、`hdiutil verify <dmg>` | mygo `cmd/mygo/bundle.go:189`–`222`、`cmd/mygo/dmg.go:95`–`100`；实测两条都通过 |
| 23 ✏️ | "mygo 文档很薄，源码就是文档" | **不成立**：模块里有 `docs/ui/*.md` 71 篇约 4100 行（一个控件一篇 + layout/styling/testing/transitions/rendering/views/windows），根目录还有 native/windows/configuration/distribution，`examples/gallery` 是控件总览 | 先读 `$(go env GOMODCACHE)/github.com/egoist/mygo@v0.3.4/docs/ui/`，再看源码；缓存里没有就先 `go mod download github.com/egoist/mygo` | mygo `docs/ui/`（71 个文件）、`docs/native.md`、`examples/gallery/main.go` |
| 24 ✅ | 纯 Go、无 cgo，不需要显示器 | — | `go test ./internal/gui/` 直接跑；`CGO_ENABLED=0` 一样通过 | 实测 `CGO_ENABLED=0 go test -count=1 ./internal/gui/` → `ok`；`ui` 包无 cgo，系统调用走 purego |
| 25 ✏️ | 真窗口截图要屏幕录制权限 | 只有系统的 `screencapture` 要；`LEEWAY_GUI_CAPTURE` 走 `Window.CapturePage()`，对 `Content` 窗口是自己重新栅格化，**不需要**权限 | 用 `LEEWAY_GUI_CAPTURE`（先重新 build 出 `.app`），或者人工看 | mygo `window.go:1085`–`1104`、`ui/window.go:343`–`355`、`content.go:184`–`209`；本仓库 `internal/gui/app.go:183`–`198` |
| 26 🆕 | 快照两次运行不完全逐字节相同 | 还在跑颜色过渡的区域会漂 1–2 个最低位 | 别用逐字节相等断言快照；用带阈值的 `meanDifference` | 实测 32 张里 1 张差 13745 像素、最大通道差 2；本仓库 `internal/gui/gui_test.go:1266`–`1268`、`:1261`–`1274` |
| 27 🆕 | 只想让某个元素"占满剩下高度"，不知道写 `Grow` 还是 `FillHeight` | 两者语义不同：`Grow(1)` 分剩余空间（按父容器主轴），`FillHeight` = 父容器 content box 的 100% | Column 里 `Grow(1)` 就够；Row 里 `Grow` 管宽度，高度必须靠 `FillHeight()` 或 `AlignItems(Stretch)`，且 Row 要有确定高度 | mygo `docs/ui/layout.md:33`–`41`、`ui/layout.go:513`–`565`；实测 300 ✓ / 619.3 ✗ / 300 ✓ |
| 28 🆕 | 想知道"这个控件在测试里点得到吗" | `Find`/`Click` 只认可见元素，被裁掉的返回 `W=0 H=0` | 断言前先看 `Find` 的 Rect；不可见就先 `navigate` 到那一页或先 `Scroll` | mygo `ui/runtime.go:804`–`837`、`ui/headless.go:227`–`264`；实测 `{W:0 H:0}` vs `{W:46.7 H:14.1}` |

## 6. 下次加一个页面 / 一个控件

### 6.1 加一个页面（最小骨架）

四个改动点，都是现有写法。假设要加"对比"页：

```go
// 1) internal/gui/page_compare.go —— 新页面：pageHead + card，一块接一块直接写（没有入场动画）
package gui

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// comparePage 是……
func (a *App) comparePage(c *ui.Context, p palette) {
	pageHead(c, p, "对比", "把几个账号放在一起看", func() {
		if ui.PrimaryButton(c, "刷新").Disabled(a.fetching).Clicked() {
			a.Refresh(true)
		}
	})
	card(c, p, func() {
			section(c, p, "按窗口")
			ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Start).Children(func() {
				for i, v := range a.providers() {
					if i > 0 {
						columnRule(c, p)                       // 1px 竖线，靠 AlignSelf(Stretch) 贯通
					}
				ui.Column(c).Grow(1).MinWidth(0).Gap(7).Children(func() {  // 等宽可压缩列
					ui.Text(c, v.DisplayName).FontSize(sizeBody).FontWeight(600).SingleLine()
					muted(c, p, fmt.Sprintf("%d 个账号", len(v.Accounts)))
				})
			}
		})
	})
}
```

```go
// 2) internal/gui/view.go:352 的 pageView 里加一个 case
func (a *App) pageView(c *ui.Context, p palette) {
	switch {
	case a.page == "compare":
		a.comparePage(c, p)
	// …原有的 history / about / provider:…
	}
}

// 3) internal/gui/view.go 的 sidebarItems 里加一行（第一个参数就是 a.page 的值）。
//    用 sidebarItem() 而不是 ui.SidebarItem()：非当前页的图标不该是强调色，
//    最后一个参数是「这一页是不是当前的」。
sidebarItem(c, "compare", iconCompare, "对比", a.page == "compare")  // iconCompare 在 icons.go 里用 icon(`<svg …>`) 定义

// 4) internal/gui/gui_test.go —— 内容 + 版式各一条
func TestComparePageShowsEveryProvider(t *testing.T) {
	a := testApp(t)
	a.navigate("compare")                                    // navigate 会存 LastPage 并 Announce 页名
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	for _, v := range a.providers() {
		if !tester.HasText(v.DisplayName) {
			t.Errorf("对比页没有 %s；texts=%v", v.DisplayName, tester.Texts())
		}
	}
	snapshot(t, tester, "compare")                           // 只在 LEEWAY_GUI_SNAPSHOT 下写 PNG
}
```

要点：**页面直接按顺序写块，不包任何入场 holder**（入场编排已删，见 §3）；
`pageHead` 给页面标题和动作（`internal/gui/view.go:366`–`369`）；卡片一律 `card(c, p, fn)`；
页面里所有"现在"都用 `a.now()`，测试把 `a.clock` 钉在 `pinned` 上才能出稳定快照
（`internal/gui/app.go:58`–`60`、`internal/gui/gui_test.go:48`）。

### 6.2 加一个控件（最小骨架）

```go
// 有文字的控件：直接给 label，测试点得到（Find/Click 精确匹配 text 或 Label）
if ui.PrimaryButton(c, "保存").Disabled(a.fetching).Clicked() {
	a.settings.save()
}

// 纯图标按钮：必须给 Label，否则测试点不到（窗口里每个纯图标按钮都自己给了 Label）
ui.Button(c, "").Label("刷新").Padding(6, 9).Radius(8).Children(func() {
	ui.Icon(c, iconRefresh).FontSize(15).TextColor(p.t.TextMuted)
}).Clicked()

// 下拉/菜单：内容抽成纯函数，端到端用 Tester.Menu() 验
ui.MenuButton(c, loginButtonLabel(id), func(m *ui.Menu) {
	for _, method := range loginMethods(id) {
		if m.Item(method.label).Chosen() {
			method.open(a, id, account)
		}
	}
})  // 几何交给 btnSmall / btnLarge，不再各写各的

// 数值 + 条：数字和条共用同一个 Animate，刷新时一起走
value(c, p, amountText(w), p.amountColor(w))       // FontFeatures("tnum") 等宽数字
bar(c, p, fraction, barColor(p, w), 7)             // internal/gui/theme.go 的 bar()

// 需要量宽度再决定文案长短：在构建期量，别猜
if w, _ := c.MeasureText(0, ui.Span{Text: label, Size: sizeBody}); w > columnWidth {
	label = shortLabel
}
```

三条纪律：

1. **可点的东西必须有唯一、可读的 label**（`Find`/`Click` 精确匹配，见坑 12）。
2. **放不下就自己写短或挂 `Tooltip`**，别指望 mygo 帮你排版（坑 13、14）。
3. **列里的内容要能被压缩**：容器 `Grow(1).MinWidth(0)`，文字 `SingleLine()`/`MaxLines(n)`（坑 5）。

## 主题：颜色是构建期读的，`SetTheme` 不是作用域

`coreSidebarItem` 把每一项的图标写死成 `t.Accent`（`ui/sidebar.go:187` 一带的
`iconColor := t.Accent`），外面没有参数可以改。想只改这一处的配色，利用「widget 的颜色是**构建时**
从 `c.Theme()` 读进 node 的」这一点：套一份主题副本，构建完再换回来。

```go
full := c.Theme()          // *ui.Theme
muted := *full             // 复制值，别改原对象
muted.Accent = full.TextMuted
c.SetTheme(&muted)
item := ui.SidebarItem(c, id, icon, label)
c.SetTheme(full)           // 必须还原
```

三个要点：

1. `SetTheme` **不是作用域**：它改的是这一帧的 `c.theme`，还会顺手写 `c.root.bg` 和根文字样式
   （`ui/context.go` 的 `SetTheme`），所以必须还原，否则整窗背景跟着变。
2. 只影响**构建期**读色的 widget。`DrawOver`/`Draw` 回调里用 painter 的主题，那是**绘制期**的，
   作用域已经退出，拿到的是原主题。
3. 复制的是值（`*full`），不要在原 `*Theme` 上改字段——那会污染全局主题。

同理可推：要问「这个颜色从哪来」，先分清它是构建期写进 node 的，还是绘制期从主题读的。

## Toast 有 Title 和 Description（我们只用 Title）

**本仓库的用法**：toast 只说应用自己写的那句话（`Title`）。`Description` 那一行 MyGo 画在
`text.Alpha(0.75)`，而「库的原文不上屏」是 `failureText()` 立下的规矩，所以这里不填它：
`failureText(sentence, message)` 的第二个返回值**恰好就是**库原文（带路径、冒号、`exit status`
这类机器格式的，或者没有中文的，都不进句子），拿到就丢：**toast 那几条路**先 `log.Printf` 到
stderr 再丢，要精确错误的人本来就在看终端；**凭据弹层那五条路**不写日志，原文只挂在
`errorLine` 的 tooltip 上（tooltip 会作为元素描述读给辅助技术，所以那条路也没丢信息）。

API 本身是 `c.Toast(text)`（简写，只有一行）与 `c.AddToast(ui.Toast{Title, Description, Type})`
（两行，Description 小一号、75% 透明度）。**Description 不是「更详细的错误」的位置**：它是给
「一句话说不完、但两句都值得看」的场合准备的，而错误原文两行都不该上屏。

## `Tester.Frame()` 会 settle，比不出「在途帧」

`Frame()` 内部 `settle()` 一直画到视图不再请求为止（最多 20 帧，`ui/headless.go:159-170`）。
所以「首帧 vs 稳定帧」这类断言**两边都是稳定帧**：把 `panelMotion.Enter` 的 Y 从 8 改成 200、
Duration 从 240ms 改成 2000ms，测试照样绿。要看在途帧，得有一个有帧上限的渲染入口，MyGo
v0.3.4 没有。本仓库因此把那条测试的措辞改成它真能证明的事（首帧已经有数字）。

## `Find` 返回第一个匹配

给开关加 `.Label(name)` 之后，同一行里「文字标签」和「开关」同名，`Tester.Find(name)` 会命中先
构建的那个（文字），量出来的是文字框不是开关的点击区。量点击区要么用 `Find` 拿到唯一名字
（`NumberInput` 的 `Increase`/`Decrease` 就是唯一名字），要么按几何直接 `ClickAt` 探边界。

## 焦点：`Focused()` 需要窗口有焦点，探针也可能测了个空

两个坑，加起来能骗掉半天：

1. `node.Focused()` 是 `hasState() && rt.focused == e.id && rt.windowFocused`
   （`ui/input.go`）——**窗口自己没有焦点时它永远是 false**。headless 里要先
   `tester.SetFocused(true)`，否则「元素能不能拿到焦点」这件事根本测不出来。
2. `Tester.Focused(s)` 的语义是「显示 s 的元素**或它的容器**有焦点」，所以它不能证明
   焦点在某个具体元素上。要判断「谁拿到了焦点」，用会被写入的那份状态（本仓库是
   `a.scrub`），或者 `Find` 一个只有那个元素才有的名字。
3. 还有一个更钝的坑：**探针跑在一个没有渲染出那个控件的 fixture 上**。总览的趋势图只在
   `len(points) >= 2` 时构建，而默认 fixture 的 hero 卡里没有历史点——于是
   「图不在 tab 顺序里」这个结论，其实是「图上根本没有图」。渲染快照能一眼看出这件事，
   探针日志不能：先看快照里有没有那个东西，再去测它的行为。

`Tester.Find` 走的是 `rt.labels`，只有 `label != ""`（或文本元素）且**未被判为 invisible**
的节点才在里面，所以 `Find` 找不到一个带 label 的元素，等于说它在提交时被判成了 invisible
（`ui/runtime.go` 的 `commitElement`），这本身就是一条线索。
