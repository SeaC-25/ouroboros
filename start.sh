#!/bin/bash

echo "🚀 启动 Ouroboros 完整系统"
echo "================================"

# 加载环境变量
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/.env" ]; then
    set -a
    source "$SCRIPT_DIR/.env"
    set +a
    echo "✅ 已加载 .env 环境变量"
fi

# 检查是否已有进程运行
if pgrep -f "ouro serve" > /dev/null; then
    echo "⚠️  后端已在运行，先停止..."
    pkill -f "ouro serve"
    sleep 2
fi

if pgrep -f "npm run dev" > /dev/null; then
    echo "⚠️  前端已在运行，先停止..."
    pkill -f "npm run dev"
    pkill -f electron
    sleep 2
fi

# 启动后端
echo ""
echo "📡 启动后端API服务器..."
cd "$SCRIPT_DIR"
nohup ./bin/ouro serve test-workspace --port 8080 > /tmp/ouro-api.log 2>&1 &
BACKEND_PID=$!
echo "   后端PID: $BACKEND_PID"

# 等待后端启动
sleep 3

# 验证后端
if curl -s http://localhost:8080/api/health > /dev/null; then
    echo "   ✅ 后端启动成功 (http://localhost:8080)"
else
    echo "   ❌ 后端启动失败，查看日志: tail -f /tmp/ouro-api.log"
    exit 1
fi

# 启动前端
echo ""
echo "🖥️  启动前端GUI..."
cd "$SCRIPT_DIR/gui"
nohup npm run dev > /tmp/ouro-gui.log 2>&1 &
FRONTEND_PID=$!
echo "   前端PID: $FRONTEND_PID"

echo ""
echo "================================"
echo "✅ Ouroboros 启动完成！"
echo ""
echo "📍 访问地址:"
echo "   - API服务器: http://localhost:8080"
echo "   - GUI界面: Electron窗口（自动打开）"
echo ""
echo "📋 查看日志:"
echo "   - 后端: tail -f /tmp/ouro-api.log"
echo "   - 前端: tail -f /tmp/ouro-gui.log"
echo ""
echo "🛑 停止服务:"
echo "   - pkill -f 'ouro serve'"
echo "   - pkill -f electron"
echo ""
