# 构建阶段
FROM node:20-alpine AS builder

WORKDIR /app

# 设置国内代理
ENV NPM_CONFIG_REGISTRY=https://registry.npmmirror.com

COPY package.json ./package.json
COPY package-lock.json ./package-lock.json
RUN npm ci

COPY . .
RUN npm run build

# 生产阶段
FROM nginx:alpine

WORKDIR /usr/share/nginx/html

# 删除默认配置避免与自定义配置冲突
RUN rm -f /etc/nginx/conf.d/default.conf

COPY --from=builder /app/dist .

# 配置 nginx
COPY nginx.conf /etc/nginx/nginx.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
