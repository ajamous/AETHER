# Aether admin UI image. Multi-stage Next.js standalone build.

FROM node:20-alpine AS deps
WORKDIR /app
COPY ui/admin/package.json ui/admin/package-lock.json* ./
RUN npm ci || npm install

FROM node:20-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY ui/admin/ ./
RUN npm run build

FROM node:20-alpine AS runtime
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
