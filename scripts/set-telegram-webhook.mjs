const token = process.env.TELEGRAM_BOT_TOKEN;
const secret = process.env.TELEGRAM_WEBHOOK_SECRET;
const url = process.env.TELEGRAM_WEBHOOK_URL;

if (!token || !secret || !url) {
  throw new Error("Configure TELEGRAM_BOT_TOKEN, TELEGRAM_WEBHOOK_SECRET e TELEGRAM_WEBHOOK_URL.");
}
if (!/^https:\/\//.test(url)) throw new Error("TELEGRAM_WEBHOOK_URL precisa usar HTTPS.");

const response = await fetch(`https://api.telegram.org/bot${token}/setWebhook`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ url, secret_token: secret, allowed_updates: ["message"] }),
});
const result = await response.json();
if (!response.ok || !result.ok) throw new Error("Telegram não aceitou a configuração do webhook.");
process.stdout.write("Webhook do Telegram configurado.\n");
