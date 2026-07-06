# syntax=docker/dockerfile:1

FROM node:24-alpine AS build
WORKDIR /app
COPY slides/package.json ./
RUN npm install
COPY slides/ ./
# Built at the origin root (--base /). Slidev's client router prepends BASE_URL to
# every in-deck path, so a non-root base breaks navigation — hence the deck gets
# its own origin (a dedicated port) rather than a /slides/ subpath.
RUN npm run build

FROM nginx:alpine
RUN cat > /etc/nginx/conf.d/default.conf <<'EOF'
server {
  listen 80;
  root /usr/share/nginx/html;
  location / {
    try_files $uri $uri/ /index.html;
  }
}
EOF
COPY --from=build /app/dist /usr/share/nginx/html
