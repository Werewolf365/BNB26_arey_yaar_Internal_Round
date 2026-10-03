# Quorum web image: builds apps/web (Next.js) and serves it on :3000.
FROM node:20-alpine AS build
WORKDIR /app
COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci
COPY apps/web/ ./
ARG NEXT_PUBLIC_API_BASE=http://localhost:8080
ENV NEXT_PUBLIC_API_BASE=$NEXT_PUBLIC_API_BASE
RUN npm run build

FROM node:20-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /app/package.json /app/package-lock.json ./
RUN npm ci --omit=dev
COPY --from=build /app/.next ./.next
COPY --from=build /app/next.config.js ./next.config.js
USER node
EXPOSE 3000
HEALTHCHECK --interval=15s --timeout=5s --retries=10 \
  CMD wget -qO- http://localhost:3000/ > /dev/null || exit 1
CMD ["npm", "run", "start"]
