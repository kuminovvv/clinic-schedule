FROM node:22-alpine AS deps

WORKDIR /app

# Keep pnpm aligned with packageManager and the committed lockfile.
RUN npm install --global pnpm@10.23.0

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

FROM deps AS builder

COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
# The public directory can be empty and therefore absent from a Git checkout.
RUN mkdir -p public && pnpm run build

FROM node:22-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0 \
    BACKEND_INTERNAL_URL=http://backend:8080 \
    NODE_OPTIONS=--max-old-space-size=512

RUN addgroup --system --gid 1001 nodejs \
    && adduser --system --uid 1001 nextjs \
    && mkdir -p /app/.next \
    && chown nextjs:nodejs /app/.next

# next/font/local emits the local Geist font files to .next/static/media.
COPY --from=builder --chown=nextjs:nodejs /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD node -e "const http = require('node:http'); const request = http.get('http://127.0.0.1:3000/', (res) => { res.resume(); process.exit(res.statusCode === 200 ? 0 : 1); }); request.setTimeout(4000, () => request.destroy()); request.on('error', () => process.exit(1));"

CMD ["node", "server.js"]
