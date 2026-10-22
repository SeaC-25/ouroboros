# 红黑主题修复记录

当前产品视觉以 `F:\Ouroboros\ui-designs\` 设计稿为准，并统一为红黑 Ouroboros 品牌系统。

## 颜色角色

- 黑色及红黑偏色表面：应用背景、工作区、面板和编辑器。
- `#ef3f49` 红色：品牌、主操作、焦点、危险和关键发现。
- 绿色：仅用于成功、已连接、就绪和完成等语义状态。
- 中性灰：信息、选择态和图谱辅助元素；不再使用蓝绿色作为主视觉。

## 实现位置

- `gui/src/styles/ouroboros-ide.css`：实际渲染器设计系统。
- `gui/tailwind.config.js`：兼容配置，颜色 token 映射到红黑 CSS 变量。
- `gui/src/app/OuroborosIDE.tsx`：唯一主渲染 shell。
- `gui/src/main/`：Electron 桌面窗口、preload bridge 和 Go sidecar 生命周期。

## 验证

```powershell
cd F:\Ouroboros\Ouroboros\gui
npm run test:product-contract
npm run test:desktop-contract
npm run package:desktop:dir
npm run smoke:desktop-package:runtime:quick
```

最后一条 smoke 会验证打包 Electron 窗口、真实 preload bridge、Go 后端健康状态和工作区引导数据。
