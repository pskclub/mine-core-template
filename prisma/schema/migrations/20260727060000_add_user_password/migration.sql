-- AlterTable
--
-- Added with a default so the statement succeeds on a table that already has
-- rows, then dropped so every new row must supply one. Existing rows keep an
-- empty string, which is not a valid bcrypt hash and can never match a password
-- — those accounts cannot sign in until one is set.
ALTER TABLE "users" ADD COLUMN "password" TEXT NOT NULL DEFAULT '';
ALTER TABLE "users" ALTER COLUMN "password" DROP DEFAULT;

-- CreateTable
--
-- Only the SHA-256 digest of a token is stored, never the token: a database that
-- leaks exposes no usable credentials. Rows are also what make a token
-- revocable — deleting one signs that session out immediately.
CREATE TABLE "access_tokens" (
    "id" UUID NOT NULL,
    "user_id" UUID NOT NULL,
    "token_hash" TEXT NOT NULL,
    "expires_at" TIMESTAMP(3) NOT NULL,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "deleted_at" TIMESTAMP(3),

    CONSTRAINT "access_tokens_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
-- Unique: a lookup on every authenticated request must be an index hit.
CREATE UNIQUE INDEX "access_tokens_token_hash_key" ON "access_tokens"("token_hash");

-- CreateIndex
-- Revoking every session for one user, and cleaning up expired rows.
CREATE INDEX "access_tokens_user_id_idx" ON "access_tokens"("user_id");
