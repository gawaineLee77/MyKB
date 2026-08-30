FROM node:24-alpine AS builder

WORKDIR /build/frontend

COPY upstream/weknora/frontend/package.json upstream/weknora/frontend/package-lock.json ./
COPY upstream/weknora/frontend/packages ./packages
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund \
      --fetch-retries=5 \
      --fetch-retry-mintimeout=10000 \
      --fetch-retry-maxtimeout=120000

COPY upstream/weknora/frontend/ ./
COPY branding/mindcreek /product/branding/mindcreek
COPY tools/frontend-overlay /product/tools/frontend-overlay

ENV VITE_IS_DOCKER=true
ENV NODE_OPTIONS=--max-old-space-size=4096
ARG VITE_FRONTEND_COMMIT=weknora-v0.7.2-mindcreek-stage1
ENV VITE_FRONTEND_COMMIT=${VITE_FRONTEND_COMMIT}

RUN node /product/tools/frontend-overlay/apply.mjs /build/frontend /product/branding/mindcreek \
    && npm test \
    && npm run type-check \
    && npm run build

FROM nginx:1.30.3-alpine@sha256:0d3b80406a13a767339fbe2f41406d6c7da727ab89cf8fae399e81f780f814d1

ARG MINDCREEK_UI_VERSION=0.1.0
LABEL org.opencontainers.image.title="MindCreek UI" \
      org.opencontainers.image.version="${MINDCREEK_UI_VERSION}" \
      org.opencontainers.image.source="https://github.com/gawaineLee77/MyKB" \
      org.opencontainers.image.licenses="MIT"

COPY --from=builder /build/frontend/dist /usr/share/nginx/html
COPY --from=builder /build/frontend/nginx.conf /etc/nginx/templates/default.conf.template
COPY upstream/weknora/frontend/docker-entrypoint.sh /docker-entrypoint.sh
COPY upstream/weknora/LICENSE /usr/share/licenses/MindCreek/WeKnora-LICENSE
RUN chmod +x /docker-entrypoint.sh

ENV MAX_FILE_SIZE_MB=50
EXPOSE 80
ENTRYPOINT ["/docker-entrypoint.sh"]
