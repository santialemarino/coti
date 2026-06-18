# syntax=docker/dockerfile:1

FROM node:22-alpine AS base
WORKDIR /app
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
ENV NEXT_TELEMETRY_DISABLED=1
RUN corepack enable

FROM base AS deps
COPY pnpm-workspace.yaml pnpm-lock.yaml package.json ./
COPY apps/backoffice/package.json ./apps/backoffice/
COPY apps/webapp/package.json ./apps/webapp/
COPY apps/api/package.json ./apps/api/
COPY packages/ui/package.json ./packages/ui/
COPY packages/eslint-config/package.json ./packages/eslint-config/
COPY packages/typescript-config/package.json ./packages/typescript-config/
RUN pnpm install --frozen-lockfile

FROM base AS builder
ARG NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_API_URL=${NEXT_PUBLIC_API_URL}
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/apps/backoffice/node_modules ./apps/backoffice/node_modules
COPY --from=deps /app/packages/ui/node_modules ./packages/ui/node_modules
COPY packages/typescript-config ./packages/typescript-config
COPY packages/eslint-config ./packages/eslint-config
COPY packages/ui ./packages/ui
COPY apps/backoffice ./apps/backoffice
COPY package.json pnpm-workspace.yaml turbo.json ./
RUN pnpm --filter @repo/ui run build
RUN pnpm --filter backoffice run build

FROM node:22-alpine AS runner
WORKDIR /app/apps/backoffice
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ARG NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_API_URL=${NEXT_PUBLIC_API_URL}
RUN corepack enable
COPY --from=builder /app/apps/backoffice/.next ./.next
COPY --from=builder /app/apps/backoffice/public ./public
COPY --from=builder /app/apps/backoffice/package.json ./
COPY --from=builder /app/apps/backoffice/next.config.js ./next.config.js
COPY --from=deps /app/node_modules /app/node_modules
COPY --from=deps /app/apps/backoffice/node_modules ./node_modules
COPY --from=builder /app/packages/ui /app/packages/ui
EXPOSE 3000
ENV PORT=3000
CMD ["pnpm", "start"]
