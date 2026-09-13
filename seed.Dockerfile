FROM oven/bun:1.4.2-alpine

WORKDIR /app

COPY ./package.json /app
COPY ./bun.lock /app
RUN bun install --frozen-lockfile
COPY ./prisma.config.ts /app
COPY ./prisma /app/prisma

# prisma/seed.ts imports @prisma/client, which exists only once generated. The
# schema arrives after `bun install`, so the install-time hook cannot do it.
# The URL here is a build-time placeholder — generate never connects; the real
# one comes from DATABASE_URL at run time.
RUN DATABASE_URL="postgresql://placeholder:placeholder@localhost:5432/placeholder" \
    bunx prisma generate

CMD ["bun", "run", "seed"]
