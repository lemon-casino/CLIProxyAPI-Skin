# skin-center · CLIProxyAPI 皮肤插件

CLIProxyAPI 的管理面板皮肤中心插件：在管理面板侧边栏提供「皮肤中心」入口，
内置六套主题（极光夜 / 墨夜 / 樱语 / 薄荷 / 落日 / 素笺），支持实时预览与切换记忆。

## 解耦设计（三层）

1. **与宿主解耦**：走 CLIProxyAPI 的 C ABI v1（`cliproxy_plugin_init`），不 import
   CLIProxyAPI 模块，`go.mod` 零依赖；宿主升级不影响本插件，反之亦然。
2. **与面板解耦**：不修改 `management.html`（该资产由宿主每 3 小时自动更新并做哈希校验），
   仅通过官方 `ResourceRoute` 扩展点提供独立页面，注册于
   `/v0/resource/plugins/skin-center/skin`。
3. **资产与二进制解耦**：`.so` 只是稳定适配器，皮肤页面按请求顺序解析：
   - 环境变量 `SKIN_CENTER_PAGE` 指定的绝对路径
   - 服务进程工作目录下的 `plugins/skin-center.html`（随插件一起部署）
   - 编译期内置页面（兜底，永不 404）

   换皮肤 = 编辑 `plugins/skin-center.html`，**无需重编译、无需重启**。
   响应头 `X-Skin-Source: file|embedded` 标识当前来源。

## 文件

```
main.go                 # C ABI 适配器：plugin.register / management.register / management.handle
skin_html.go            # 编译期内置的兜底页面
assets/skin-center.html # 与内置页面同源的外置资产，部署到 plugins 目录
go.mod                  # 零依赖，module skin-center
```

## 构建（Linux，与官方镜像同源环境）

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.26-bookworm \
  go build -buildmode=c-shared -o /src/skin-center.so .
```

## 部署

1. `skin-center.so` 与 `skin-center.html` 放入插件目录（容器内 `/CLIProxyAPI/plugins`）。
2. `config.yaml`：
   ```yaml
   plugins:
     enabled: true
     dir: "plugins"
     configs:
       skin-center:
         enabled: true
   ```
3. 重启服务；面板侧边栏出现「皮肤中心」，或直接访问
   `/v0/resource/plugins/skin-center/skin`（该路由无管理鉴权，页面不含任何密钥或敏感数据）。

## 登录页定制（v1.3.0）

皮肤中心页新增「登录页定制」表单，设置保存在浏览器 localStorage（`cpaSkinLogin`），
由注入脚本在登录页实时应用（每 2s 自愈，防 SPA 重渲染）：

- **品牌大标题**：替换登录页左侧 "CLI PROXY API" 大字（空格分词，2~6 个词自动适配，多余原生词隐藏）
- **登录卡片标题**：同步替换卡片标题与浏览器标签页标题（默认 "CLI Proxy API Management Center"）
- **隐藏「当前地址」提示块**（connectionBox）
- **隐藏「自定义连接地址」开关**（toggleAdvanced）

选择器使用 CSS Modules 语义前缀（`[class*=LoginPage-module__brandWord]` 等），
类名哈希后缀变化不影响；面板非登录路由自动跳过，不干扰主界面。
留空/恢复默认即还原官方原文（原始文本缓存在 data-cpa-orig）。

## 主题明暗自动同步（v1.3.1）

修复"极光夜/墨夜等暗色主题侧边栏没变色"：根因是面板停留在浅色模式
（html data-theme="white"）时，暗色主题只能套用其浅色变体（与原版色差极小）。
现在注入脚本会随主题自动同步面板明暗：暗色系（极光夜/墨夜/落日）→ data-theme=dark，
浅色系（樱语/薄荷/素笺）→ white。选皮肤即一键换装（含明暗与侧边栏）。
注意：注入脚本更新后需刷新已打开的面板页面才会加载新脚本。

## 零闪变与品牌图标（v1.4.0）

- **进页即定制态**：隐藏类定制改为 html 标记 + CSS 属性选择器（脚本解析期生效，首帧即隐藏）；
  文字类（品牌词/标题/图标）由 MutationObserver 在 DOM 插入的同一帧应用（先于浏览器绘制），
  不再出现"先原版后变"的闪变。
- **品牌图标上传**：皮肤中心表单新增图标上传（任意图片自动缩放到 160px 存 PNG dataURL，
  存 localStorage `cpaSkinLogo`），同时替换登录页 Logo（`img[class*=LoginPage-module__logo]`）
  与浏览器标签页 favicon（`link[rel*=icon]`）；「清除图标」或「恢复默认」还原官方图标。
- 已打开的页面需刷新才会加载新版注入脚本（运行中的脚本不热替换）。

## 登录页风格增强（v1.5.0）

参考 synterolink 注册页的设计语言，为暗色模式登录页增加可选风格增强
（默认开启，皮肤中心「暗色登录页风格增强」复选框控制，仅暗色主题生效）：

- 背景叠加细网格纹理（双方向 46px 间距）与主题色辉光
- 登录卡毛玻璃化：半透明 + backdrop-blur(20px) + 细边框 + 24px 大圆角 + 深阴影内高光
- 输入框半透明深填充、13px 圆角，聚焦时主题色描边 + 辉光 ring
- 登录按钮主题色→青蓝渐变、14px 圆角、常驻辉光、hover 抬升增亮
- 品牌大字改为纵向渐变文字（text-primary → primary-color），副标题加字距

全部为注入 CSS（跟随所选主题变量），不改面板文件、不碰功能逻辑。

## 品牌区 API 调用动画（v1.6.0，v1.6.1 放大）

参考 synterolink 左半区的 gateway 演示面板，在暗色登录页品牌大字下方注入
自愈式特效面板（React 清除后由 observer 自动重建），皮肤中心「品牌区 API
调用动画」复选框控制（默认开，cfg.brandFx）：

- OPENAI / CLAUDE / GEMINI / CODEX 能力标签胶囊
- LIVE GATEWAY 卡片：呼吸绿点 + 动态当前访问地址（location.host）+ 呼吸式 200 OK 徽章
- 代码区三行（POST /v1/chat/completions、Authorization 掩码、model 列表）逐行淡入循环 + 闪烁光标
- OpenAI compatible / Claude compatible / Streaming responses / 负载均衡 能力胶囊

全部主题变量取色，跟随所选皮肤。v1.6.1~v1.6.4 尺寸流式化与对齐：特效面板 width:100% 与品牌大字容器（brandContent）完全同宽同起点对齐；字号/间距/徽章全部 clamp() 按视口流式缩放（vw）。v1.6.4 修复品牌大字左溢出：原字号 280px 固定值导致宽词溢出容器（右对齐布局下向左出界被裁切），改为 clamp(88px,11vw,220px) 流式字号，大字右缘与特效面板右缘精确对齐。

## 服务器入口与根路径行为（nginx，宿主外配置）

`GET /` 返回的 `{"message":"CLI Proxy API Server",...}` 是宿主二进制硬编码的显式路由，
插件（noRoute 挂载）与面板文件都无法改变。服务器已有 nginx（80 端口），部署了智能入口：

- **`http://192.168.0.6/`（80 端口，不带 :8317）**：
  - 开关开（当前默认开）：302 跳转 `/management.html#/login`
  - 开关关：透传宿主 JSON，且 message 被替换为登录卡片标题（nginx sub_filter 静态文本；
    修改标题后需同步改 `/etc/nginx/sites-available/cliproxy.conf` 里的替换文本）
- API 调用 / 面板 / WebSocket 全部透传 8317；`:8317` 直连行为完全不变（仍是宿主原 JSON）
- 开关工具：`cliproxy-root-redirect on|off|status`（本质是标志文件
  `/etc/nginx/cliproxy-root-redirect.enabled` 的存在与否，逐请求生效，无需 reload）
- 配置：`/etc/nginx/sites-available/cliproxy.conf`（软链到 sites-enabled）、
  `/etc/nginx/conf.d/00-cliproxy-upgrade.conf`（WebSocket upgrade map）
- **缓存一致性**：宿主 serve 面板无 Cache-Control 头，8317 与 80 两域名浏览器缓存独立，
  会各自缓存"官方面板覆盖后 / 插件重注入前"窗口内的版本而显示不一致。80 入口已对
  `/management.html` 加 `Cache-Control: no-cache`（协商缓存，mtime 变化即刻拿新）根治；
  8317 直连响应头无法改（宿主 c.File），建议浏览器入口统一走 80，8317 留给 API 客户端。
  两个端口的服务器侧内容字节级一致（同一磁盘文件）。

## 站点级配置持久化（v1.7.0）

解决"清空浏览器数据后定制全部丢失"：配置提升为**站点级（服务器端）默认**，浏览器值只是覆盖层。

**数据流**：皮肤中心「把当前设置保存为站点默认」（输入管理密钥）→ 宿主标准接口
`PATCH /v0/management/plugins/skin-center/config`（管理鉴权）→ 写入 config.yaml 的
`plugins.configs.skin-center.site-config`（base64 JSON）→ 宿主 watcher 热更新并以
`plugin.reconfigure` 把配置子树推给插件 → 插件经无认证资源路由
`GET /v0/resource/plugins/skin-center/config` 输出 JSON。

**应用策略**（注入块启动时 pullSiteConfig）：localStorage 缺失的字段自动用服务器值回填
并写回本地；已有浏览器值不覆盖（保留设备级个性）。效果：清空浏览器数据/换新设备 →
首次打开面板自动恢复站点默认配置。

**皮肤中心新增**：管理密钥输入 + 「把当前设置保存为站点默认」+「载入站点默认」
（后者可跨浏览器迁移配置）。config.yaml 即持久层，随宿主正常备份/迁移。
注意：管理密钥若选择记住，保存在浏览器 localStorage，属设备级信任。

## 闪变根治（v1.7.1）

强刷后"原版内容闪一帧"的根因：MutationObserver 回调经 requestAnimationFrame 防抖，
定制被推迟到下一帧应用，中间那帧浏览器把原版内容画了出来。修复：

- 防抖从 rAF 改为**微任务**（Promise.resolve().then）——定制在 DOM 变更的同一宏任务内、
  绘制之前应用，零闪变
- MutationObserver 增加 data-theme 属性监听：面板启动时重置明暗模式会被**同帧纠正**
  （原先靠 2 秒轮询，明暗会闪）
- applyTexts 全部写入严格幂等（内容不变不写）：否则微任务观察器会自触发形成
  无限循环饿死主线程（v1.7.1 开发中实际踩到并修复的回归）
- ensure() 所有属性写入先比较后写，防止观察器自触发

注意：注入脚本更新后已打开的页面需刷新一次才会加载新脚本。

## 跨端口/跨设备配置一致性（v1.8.0）

localStorage 按域名+端口隔离（80 与 :8317 互不可见），导致品牌图标等定制
"在 A 端口设置了 B 端口不生效"。v1.8.0 起服务器配置升级为**唯一事实源**：

- 站点配置 payload 带 `updated` 时间戳；注入块拉取时按**时间戳仲裁**：
  服务器较新 → 覆盖本地并应用；本地有更新的未同步修改 → 保留本地
- 皮肤中心「保存并应用」：若已记住管理密钥则**自动同步到服务器**（全站即时一致）；
  未记住密钥则仅本浏览器生效并提示
- 图标上传/清除、主题切换同样打本地时间戳参与仲裁
- 效果：输入一次密钥保存站点默认后，80 / 8317 / 任何新设备永远显示同一套皮肤；
  之后任意端口的修改都会自动同步到全站

## 回滚

删除 `skin-center.so`、移除 configs 条目、重启即可；或管理面板一键禁用插件。
