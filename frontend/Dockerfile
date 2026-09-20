# syntax=docker/dockerfile:1

# ---- Builder ----
FROM node:20-alpine AS builder
WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

ARG VITE_API_URL
ENV VITE_API_URL=$VITE_API_URL

RUN npm run build

# ---- Runtime ----
FROM nginx:alpine
WORKDIR /usr/share/nginx/html

# Clean default static assets
RUN rm -rf ./*

# Copy built assets
COPY --from=builder /app/dist .

# Copy nginx configuration template (envsubst runs automatically on startup)
COPY nginx.conf.template /etc/nginx/templates/default.conf.template

ENV PORT=80
EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
