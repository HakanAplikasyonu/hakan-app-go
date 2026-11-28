FROM node:22.23.5-alpine3.23 AS base

ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH

RUN corepack enable && corepack prepare pnpm@11.22.0 --activate

WORKDIR /app

FROM base AS deps

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

# WhatsApp Web uzantısının kaynağı (#780): imaj bağlamı frontend/ olduğu için
# ayrı bir bağlam olarak verilir. CI: `build-contexts: whatsapp=extensions/whatsapp`;
# elle: `docker build --build-context whatsapp=../extensions/whatsapp frontend`.
# Verilmezse bu boş aşama kullanılır ve derleme düşer (WHATSAPP_EXT_REQUIRED=1).
FROM scratch AS whatsapp

FROM base AS build

ENV NEXT_TELEMETRY_DISABLED=1
# Pakete gömülecek API adresi (uzantının sunucu adresi; gizli değil). Boşsa
# kullanıcı adresi uzantıda "Gelişmiş" altına yazar.
ARG WHATSAPP_API_URL=""
ARG WHATSAPP_PANEL_URL=""
ENV WHATSAPP_API_URL=$WHATSAPP_API_URL \
    WHATSAPP_PANEL_URL=$WHATSAPP_PANEL_URL \
    WHATSAPP_EXT_DIR=/extensions/whatsapp \
    WHATSAPP_EXT_REQUIRED=1

COPY --from=whatsapp / /extensions/whatsapp/
COPY --from=deps /app/node_modules ./node_modules
COPY . .

RUN pnpm build

FROM node:22.23.5-alpine3.23 AS runtime

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
ENV PORT=3000

RUN addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app

WORKDIR /app

COPY --from=build --chown=app:app /app/public ./public
COPY --from=build --chown=app:app /app/.next/standalone ./
COPY --from=build --chown=app:app /app/.next/static ./.next/static

USER app

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=10 \
  CMD ["node", "-e", "fetch('http://127.0.0.1:3000/health').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"]

CMD ["node", "server.js"]
