FROM node:24-alpine AS builder

WORKDIR /app

COPY frontend/package*.json ./
RUN npm ci

COPY frontend ./

ARG GOOGLE_CLIENT_ID
ARG AUTH_API_ORIGIN=/api
ENV GOOGLE_CLIENT_ID=${GOOGLE_CLIENT_ID}
ENV AUTH_API_ORIGIN=${AUTH_API_ORIGIN}

RUN npm run build

FROM nginx:1.27-alpine

COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80
