#!/bin/bash

# 本地测试脚本（不依赖Docker）

echo "=== 撮合引擎本地测试 ==="
echo ""

# 编译
echo "1. 编译项目..."
go build -o matchengine ./cmd/matchengine
if [ $? -ne 0 ]; then
    echo "✗ 编译失败"
    exit 1
fi
echo "✓ 编译成功"
echo ""

# 运行单元测试
echo "2. 运行单元测试..."
go test ./... -v -short 2>&1 | head -50
echo ""

# 运行集成测试
echo "3. 运行撮合引擎测试..."
go run test/main.go
echo ""

echo "=== 测试完成 ==="
