FROM oven/bun:1.4.2-alpine

WORKDIR /app

COPY ./package.json /app
COPY ./bun.lock /app
RUN bun install --frozen-lockfile
COPY ./prisma.config.ts /app
COPY ./prisma /app/prisma

# `migrate deploy`, not `migrate dev`: it only applies migrations already
# committed under prisma/schema/migrations (prisma keeps them beside the schema
# directory it was pointed at). `dev` authors new ones — it wants a shadow
# database and a prompt, and can reset a shared database.
CMD ["bun", "run", "migrate:deploy"]
