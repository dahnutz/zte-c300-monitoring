FROM docker.io/library/node:25-bookworm-slim AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm test && npm run build

FROM docker.io/nginxinc/nginx-unprivileged:1.27-alpine AS runtime
USER root
RUN apk add --no-cache gettext \
    && rm -f /etc/nginx/conf.d/default.conf \
    && printf 'include /tmp/default.conf;\n' > /etc/nginx/conf.d/app.conf
COPY docker-entrypoint.sh /docker-entrypoint.sh
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /src/dist /usr/share/nginx/html
RUN chmod 755 /docker-entrypoint.sh && chown -R 101:101 /usr/share/nginx/html
USER 101
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null
ENTRYPOINT ["/docker-entrypoint.sh"]
