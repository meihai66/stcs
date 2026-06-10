# syntax=docker/dockerfile:1

# ---------- 阶段 1:构建前端(Vite + React) ----------
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

# ---------- 阶段 2:构建 Go 后端(嵌入前端 dist) ----------
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 用阶段 1 产物覆盖 web/dist,供 //go:embed 嵌入
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /stcs .

# ---------- 阶段 3:最小运行镜像 ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app
COPY --from=build /stcs /app/stcs
RUN mkdir -p /app/data
# 以 root 运行,保证绑定挂载/命名卷的 /app/data 始终可写(自托管内网工具,够用)
ENV STCS_DATA_DIR=/app/data \
    STCS_PORT=5311 \
    TZ=Asia/Shanghai
EXPOSE 5311
VOLUME ["/app/data"]
ENTRYPOINT ["/app/stcs"]
