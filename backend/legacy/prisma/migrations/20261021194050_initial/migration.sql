-- CreateTable
CREATE TABLE "Integration" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "name" TEXT NOT NULL,
    "slug" TEXT NOT NULL,
    "baseUrl" TEXT NOT NULL,
    "encryptedApiKey" TEXT NOT NULL,
    "apiKeyIv" TEXT NOT NULL,
    "apiKeyAuthTag" TEXT NOT NULL,
    "active" BOOLEAN NOT NULL DEFAULT true,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" DATETIME NOT NULL
);

-- CreateTable
CREATE TABLE "ProvisioningRecord" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "idempotencyKey" TEXT NOT NULL,
    "integrationId" TEXT NOT NULL,
    "payloadHash" TEXT NOT NULL,
    "payloadCiphertext" TEXT NOT NULL,
    "payloadIv" TEXT NOT NULL,
    "payloadAuthTag" TEXT NOT NULL,
    "status" TEXT NOT NULL,
    "responseCiphertext" TEXT,
    "responseIv" TEXT,
    "responseAuthTag" TEXT,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT "ProvisioningRecord_integrationId_fkey" FOREIGN KEY ("integrationId") REFERENCES "Integration" ("id") ON DELETE RESTRICT ON UPDATE CASCADE
);

-- CreateTable
CREATE TABLE "TelegramConversation" (
    "chatId" TEXT NOT NULL PRIMARY KEY,
    "state" TEXT NOT NULL,
    "draftJson" TEXT NOT NULL,
    "updatedAt" DATETIME NOT NULL
);

-- CreateTable
CREATE TABLE "AuditLog" (
    "id" TEXT NOT NULL PRIMARY KEY,
    "action" TEXT NOT NULL,
    "integrationId" TEXT,
    "requestId" TEXT,
    "result" TEXT NOT NULL,
    "createdAt" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- CreateIndex
CREATE UNIQUE INDEX "Integration_slug_key" ON "Integration"("slug");

-- CreateIndex
CREATE UNIQUE INDEX "ProvisioningRecord_idempotencyKey_key" ON "ProvisioningRecord"("idempotencyKey");
