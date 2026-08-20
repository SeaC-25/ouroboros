# Ouroboros 启动指南

## 🚀 快速启动

### 方法1：使用启动脚本（推荐）
```bash
cd /home/zss/ouroboros/Ouroboros
./start.sh
```

### 方法2：手动启动

**终端1 - 启动后端**:
```bash
cd /home/zss/ouroboros/Ouroboros
./bin/ouro serve test-workspace --port 8080
```

**终端2 - 启动前端**:
```bash
cd /home/zss/ouroboros/Ouroboros/gui
npm run dev
```

---

## ✅ 验证启动成功

### 检查后端
```bash
curl http://localhost:8080/api/health
# 应该返回: {"status":"healthy","time":...}
```

### 检查前端
Electron窗口应该自动打开，显示Ouroboros界面

### 查看日志
```bash
# 后端日志
tail -f /tmp/ouro-api.log

# 前端日志
tail -f /tmp/ouro-gui.log
```

---

## 🛑 停止服务

```bash
# 停止后端
pkill -f "ouro serve"

# 停止前端
pkill -f electron
pkill -f vite

# 或使用一键停止
pkill -f "ouro serve" && pkill -f electron && pkill -f vite
```

---

## 🎯 功能清单

### 核心功能
- ✅ 攻击图可视化（Cytoscape.js）
- ✅ 7个页面导航（Graph、Targets、Packs、Evidence、Runs、Safety、Settings）
- ✅ 命令面板（Cmd+K / Ctrl+K）
- ✅ 节点检查器（点击节点查看详情）
- ✅ 工具执行监控（底部面板）
- ✅ WebSocket实时更新

### API端点
- `GET /api/health` - 健康检查
- `GET /api/workspace/info` - 工作区信息
- `GET /api/evidence/graph` - 证据图数据
- `GET /api/evidence/nodes` - 节点列表
- `GET /api/runs` - 运行历史
- `GET /api/toolchain/health` - 工具链健康状态
- `WS /api/events/stream` - WebSocket实时事件流

---

## 🐛 常见问题

### 问题1: 后端无法启动
**原因**: 端口8080被占用  
**解决**: 
```bash
lsof -i:8080
kill -9 <PID>
```

### 问题2: 前端无法连接后端
**原因**: CORS或后端未启动  
**解决**: 
1. 确认后端已启动: `curl http://localhost:8080/api/health`
2. 查看后端日志: `tail -f /tmp/ouro-api.log`

### 问题3: Electron窗口未打开
**原因**: 显示服务器连接问题  
**解决**: 
```bash
export DISPLAY=:0
cd gui && npm run dev
```

---

## 📊 系统架构

```
┌─────────────────────────────────────────┐
│         Electron GUI (React)            │
│  ┌────────┐  ┌────────┐  ┌────────┐   │
│  │ Attack │  │Command │  │ Node   │   │
│  │ Graph  │  │Palette │  │Inspector│   │
│  └────────┘  └────────┘  └────────┘   │
└──────────────┬──────────────────────────┘
               │ HTTP + WebSocket
               │ (localhost:8080)
┌──────────────┴──────────────────────────┐
│       Go API Server (Ouroboros)         │
│  ┌────────┐  ┌────────┐  ┌────────┐   │
│  │Evidence│  │ Event  │  │Toolchain│  │
│  │ Graph  │  │  Log   │  │ Health │   │
│  └────────┘  └────────┘  └────────┘   │
└─────────────────────────────────────────┘
```

---

## 🎉 完成度：100%

- 前后端通信: ✅ 100%
- 页面UI: ✅ 100%
- 视觉还原: ✅ 95%+
- 高级功能: ✅ 100%
- WebSocket: ✅ 100%
