# 学习平台前端镜像：Vite 构建产物 + nginx 托管
# 阶段一：安装依赖并执行构建
FROM node:22-alpine AS builder

# 国内网络环境使用 npmmirror 加速依赖下载
RUN npm config set registry https://registry.npmmirror.com
WORKDIR /build
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# 阶段二：nginx 托管静态文件，并把 /api 反向代理到后端容器
FROM nginx:1.27-alpine
COPY --from=builder /build/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
