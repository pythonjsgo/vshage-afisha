# syntax=docker/dockerfile:1
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev

FROM node:22-alpine AS builder
WORKDIR /app
# PUBLIC_API_URL is baked in by Vite at build time. Default `/api` keeps
# the image usable for `npm run build` outside CI, but production images
# are always built with an absolute URL so SvelteKit SSR fetch works.
ARG PUBLIC_API_URL=/api
ENV PUBLIC_API_URL=$PUBLIC_API_URL
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production PORT=3000 HOST=0.0.0.0
COPY --from=deps /app/node_modules ./node_modules
COPY --from=builder /app/build ./build
COPY --from=builder /app/static ./static
COPY package.json ./
EXPOSE 3000
CMD ["node", "build"]
