# syntax=docker/dockerfile:1
# Klijent (Angular, US-32). Build iz root-a repozitorijuma:
#   docker build -f client/Dockerfile -t fsp-client client
#
# 1. faza: gradi Angular aplikaciju u statičke fajlove.
# 2. faza: nginx (bez root prava) ih servira na portu 8080; config.json sa
#    adresom gateway-a pravi se pri pokretanju iz env promenljive GATEWAY_URL.

FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
RUN npx ng build --configuration production

FROM nginxinc/nginx-unprivileged:1.28-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist/client/browser /usr/share/nginx/html
COPY --chmod=755 docker/40-config-json.sh /docker-entrypoint.d/40-config-json.sh
EXPOSE 8080
