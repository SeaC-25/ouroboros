# Ouroboros 桌面 IDE 启动指南

## 运行版本

打包目录：

```text
F:\Ouroboros\Ouroboros\gui\dist\desktop\Ouroboros-win32-x64\
```

可执行文件：`Ouroboros.exe`。它会启动 Electron 桌面窗口、preload bridge 和本地 Go 后端；浏览器预览只用于开发调试。

## 启动

PowerShell：

```powershell
cd F:\Ouroboros\Ouroboros\gui\dist\desktop\Ouroboros-win32-x64
.\Ouroboros.exe
```

从源码启动：

```powershell
cd F:\Ouroboros\Ouroboros\gui
npm run dev
```

## 验收重点

- 首屏是桌面 IDE 工作区，不是营销页或普通 Web 页面。
- 主题是红黑：红色用于品牌、焦点、主操作和风险；绿色只表示成功、就绪或连接状态。
- 工作区选择、窗口控制、后端状态和文件展示都通过 Electron bridge 工作。
- 右侧攻击图谱、智能体图谱和检查器在空数据时显示明确空状态，不伪造数据。

打包并运行真实桌面 smoke：

```powershell
cd F:\Ouroboros\Ouroboros\gui
npm run package:desktop:dir
npm run smoke:desktop-package:runtime:quick
```
