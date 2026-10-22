# Ouroboros 前端重构状态

## 当前架构

Ouroboros 是 Electron 桌面 IDE，不是独立 Web 产品。唯一渲染入口为：

```text
src/renderer/main.tsx -> src/renderer/App.tsx -> src/app/OuroborosIDE.tsx
```

Electron 主进程负责窗口、preload bridge、工作区选择、Go sidecar 启停和状态同步。Vite 只负责开发服务器与生产构建；浏览器预览不代表最终产品运行时。

## 视觉系统

- 黑色/红黑偏色表面，细边框和紧凑 IDE 间距。
- 红色作为唯一品牌与主操作强调色。
- 绿色仅表示成功、就绪、连接和完成。
- 使用真实 Ouroboros 蛇形位图资源，不使用抽象占位标记。

## 已验证工作台

任务、AI、黑板、智能体、评测、攻击图谱、回放、报告、运行时和设置均从同一 shell 切换，并使用后端 DTO、真实文件和明确空/错误状态。

## 验证命令

```powershell
npm run typecheck
npm run test:contract
npm run test:product-contract
npm run test:desktop-contract
npm run package:desktop:dir
npm run smoke:desktop-package:runtime:quick
```

桌面 smoke 已验证打包后的 Electron、preload bridge、Go sidecar 和工作区数据可以完整连通。
