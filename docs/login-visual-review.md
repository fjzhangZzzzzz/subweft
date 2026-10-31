# 登录页黑白视觉实现评审

2026-10-31：已实现并完成本地自评，用户已确认视觉评审并授权提交、推送和发布 v0.3.0-rc.2；未部署生产。发布结果以对应 GitHub Release 为准。

## 实现范围

登录页使用 Go HTML 模板与本地 CSS，无 JavaScript 依赖即可显示和进入 `/auth/login`。桌面卡片宽 420px、内边距 32px、圆角 16px；手机左右留白 24px、卡片内边距 24px/20px，主操作至少 52px。原创线性 S 标志供本轮评审，未使用示意图位图。

`web/public/theme.css` 统一中性色、系统字体、主按钮和焦点颜色；编辑器沿用原导航、布局及业务流程，保留错误、警告、成功和差异的语义色。输入控件使用较深边框。

可选环境变量 `SUBWEFT_OIDC_DISPLAY_NAME` 仅控制展示文案；未设置时显示“继续登录”。名称经过模板 HTML 转义。认证错误保留原 HTTP 状态与重试入口，不展示请求参数。Cookie、OIDC 校验、CSRF 及成功回跳路径保持原行为。

## 实际渲染截图

截图来自本地构建程序及 Chromium，不是设计示意图。`output` 为忽略的本地产物，不随源码发布。

| 场景 | 本地截图 |
| --- | --- |
| 桌面 1440×900 | `output/playwright/login-review/desktop.png` |
| 手机 375×812 | `output/playwright/login-review/mobile.png` |
| 手机键盘焦点 | `output/playwright/login-review/focus.png` |
| 横屏 812×375，可滚动完整页面 | `output/playwright/login-review/landscape.png` |
| 长提供方名称 | `output/playwright/login-review/long-name.png` |
| 失败回调，HTTP 401 | `output/playwright/login-review/error.png` |
| 未授权身份，HTTP 403 | `output/playwright/login-review/access-denied.png` |
| 编辑器概览 | `output/playwright/login-review/editor.png` |
| 新增订阅与输入焦点 | `output/playwright/login-review/subscription-form.png` |
| 200% CSS 放大 | `output/playwright/login-review/zoom-200-css.png` |
| 200% 等效布局视口 720×450 | `output/playwright/login-review/zoom-200-layout.png` |

## 验证结果与边界

- 前端构建、格式检查、`go test -tags webui ./...`、`go vet ./...`、`git diff --check` 通过。
- 新增测试覆盖匿名登录页、提供方文案及 HTML 转义、错误回调不泄露参数，并继续执行成功登录及 CSRF 拒绝回归。
- `tools/editor_smoke.py` 对本次构建通过：OIDC、CSRF、原生校验、混合规则集、发布、失败保留活动版本、50 次并发下载、设备暂停恢复及 token 轮换、恢复到草稿、CLI 备份恢复。结果位于 `output/login-review/smoke.json`。
- 浏览器实测 Tab 焦点、Enter 跳转、本地 OIDC 管理员登录回到编辑器、未授权身份拒绝、失败后重试、退出返回登录页。登录页不依赖外部字体或 CDN。
- 桌面、手机、长名称、短屏自评未发现内容裁切或横向溢出；短屏自然滚动，页脚不覆盖卡片。表单边界、选中态、焦点可辨识。
- WCAG 对比度计算：正文/白底 17.57:1，说明/白底 6.14:1，页脚/背景 5.78:1，按钮文字 17.05:1，焦点/白底 7.58:1，输入边框/白底 3.87:1，错误文字 7.19:1。
- 缩放已检查 CSS 200% 放大和等效布局视口；自动化快捷键未改变浏览器原生缩放，不能将上述检查表述为原生浏览器 200% 验收。
- 本地展示 Pocket ID 名称仅用于视觉预览，实际连接独立合成 OIDC 服务。生产 Pocket ID 登录尚未验证；未读取生产凭据、修改 homelab-infra 或部署生产。

用户评审入口：`http://127.0.0.1:18904/`，使用本地测试管理员身份。默认文案预览为 `http://127.0.0.1:18906/`。这两个地址仅供本机临时评审。

视觉及原创标志已获用户确认。剩余部署侧验收：人工浏览器原生 200% 缩放、真实 Pocket ID 登录。

## UI 评审修正（第二轮）

用户认可后的三项建议均已落实：登录页、编辑器品牌与 favicon 共用 `web/public/favicon.svg`；窄屏长提供方名称使用“继续登录”，提示保留完整名称；编辑器残留蓝灰背景、分隔线与次要文字归并共享中性色，语义色保留。

真实截图：`output/playwright/login-review/desktop-v2.png`、`output/playwright/login-review/mobile-v2.png`、`output/playwright/login-review/long-name-v2.png`、`output/playwright/login-review/editor-v2.png`。320px 长名称按钮恢复单行，提示完整、无裁切，页脚可见。桌面与手机默认文案保持原样。复评上述三项通过。

本轮前端构建、格式检查、全部 Go 测试及浏览器 OIDC 登录回跳通过。用户已授权提交、推送和发布；生产 Pocket ID 与浏览器原生 200% 缩放仍待独立人工验收。
