# 多阶段构建：前端编译 + Go 静态编译 + Alpine 运行时
# 说明：上游将编译产物 www/browser 提交在仓库中；为让 Angular 源码改动（如组件模板）
# 生效，镜像构建时先用 node 编译最新前端，再覆盖到 Go 构建上下文完成内嵌
FROM node:20-alpine AS web

WORKDIR /web

COPY package.json angular.json tsconfig.json tsconfig.app.json ./
COPY src ./src
COPY public ./public
RUN npm install --no-audit --no-fund && npx ng build

FROM golang:1.25-alpine AS builder

ARG GOPROXY=https://goproxy.cn,direct
ARG VERSION=1.0.0
ARG GIT_HASH=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 用最新编译的前端覆盖仓库内嵌产物（go:embed www/browser）
COPY --from=web /web/www/browser /src/www/browser

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w \
    -X 'github.com/god-jason/iot-master/pkg/version.Name=iot-master' \
    -X 'github.com/god-jason/iot-master/pkg/version.Version=${VERSION}' \
    -X 'github.com/god-jason/iot-master/pkg/version.GitHash=${GIT_HASH}'" \
    -o /out/iot-master main.go

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1000 app && adduser -u 1000 -S -G app app

# 二进制与插件目录放 /opt/iot-master，首次启动由 initContainer 播种到持久化目录 /app-data
# 注意：二进制不能与配置查找名同名（iot-master），否则 viper 的裸文件名兜底会把 ELF 二进制
# 当作配置文件解析（报 yaml control characters）；main.go 已硬编码配置名为 iot-master，改名无副作用
COPY --from=builder /out/iot-master /opt/iot-master/iot-master-app
COPY pages /opt/iot-master/pages
COPY protocols /opt/iot-master/protocols
COPY tables /opt/iot-master/tables
# 前端侧边栏菜单由 /api/menu 从工作目录读取 menu.json，必须随镜像分发并播种到持久化目录
COPY menu.json /opt/iot-master/menu.json

RUN chown -R 1000:1000 /opt/iot-master

# 工作目录即数据目录：iot-master.yaml、pages/、protocols/、tables/、上传文件都落在这里
WORKDIR /app-data

USER 1000

EXPOSE 8080 1883

ENTRYPOINT ["/opt/iot-master/iot-master-app"]
