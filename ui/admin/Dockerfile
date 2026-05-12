# Aether admin UI image. Multi-stage Next.js standalone build.

FROM node:26-alpine AS deps
WORKDIR /app
COPY ui/admin/package.json ui/admin/package-lock.json* ./
RUN npm ci || npm install

FROM node:26-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY ui/admin/ ./
# Turbopack (Next.js 16) folds process.env reads to constants at
# build time. These ARG/ENV values are the defaults for the lab
# compose stack (container DNS names on the lab network). For other
# topologies, override via `docker build --build-arg`.
ARG AETHER_GATEWAY_URL=http://gateway:8080
ARG AETHER_AUDIT_URL=http://audit:8447
ARG AETHER_CERTMGR_URL=http://certmgr:8444
ENV AETHER_GATEWAY_URL=${AETHER_GATEWAY_URL} \
    AETHER_AUDIT_URL=${AETHER_AUDIT_URL} \
    AETHER_CERTMGR_URL=${AETHER_CERTMGR_URL}
RUN npm run build

FROM node:26-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
RUN addgroup -S aether && adduser -S aether -G aether
COPY --from=build /app/public ./public
COPY --from=build --chown=aether:aether /app/.next/standalone ./
COPY --from=build --chown=aether:aether /app/.next/static ./.next/static
USER aether
EXPOSE 3000
CMD ["node", "server.js"]
