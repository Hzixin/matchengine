# 构建阶段
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 安装依赖
RUN apk add --no-cache git make

# 复制go mod文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o matchengine ./cmd/matchengine

# 运行阶段
FROM alpine:latest

WORKDIR /app

# 安装ca证书
RUN apk --no-cache add ca-certificates tzdata

# 从构建阶段复制二进制文件
COPY --from=builder /app/matchengine /app/matchengine
COPY --from=builder /app/configs /app/configs

# 创建数据目录
RUN mkdir -p /app/data/raft

# 暴露端口
EXPOSE 8080 50051 7000

# 运行
ENTRYPOINT ["/app/matchengine"]
CMD ["-config", "/app/configs/config.yaml"]
