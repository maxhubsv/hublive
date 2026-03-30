/**
 * Client-side JWT generation for LiveKit (dev mode only).
 * In production, tokens MUST be generated server-side.
 */

function base64UrlEncode(data: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < data.length; i++) {
    binary += String.fromCharCode(data[i]!);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function strToBase64Url(str: string): string {
  return base64UrlEncode(new TextEncoder().encode(str));
}

export async function generateLiveKitToken(
  apiKey: string,
  apiSecret: string,
  identity: string,
  roomName: string,
): Promise<string> {
  const header = strToBase64Url(JSON.stringify({ alg: "HS256", typ: "JWT" }));

  const now = Math.floor(Date.now() / 1000);
  const payload = strToBase64Url(
    JSON.stringify({
      iss: apiKey,
      sub: identity,
      nbf: now,
      exp: now + 86400,
      iat: now,
      identity,
      name: identity,
      video: {
        roomJoin: true,
        room: roomName,
        canPublish: true,
        canPublishData: true,
        canSubscribe: true,
      },
    }),
  );

  const signingInput = `${header}.${payload}`;
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(apiSecret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const sig = await crypto.subtle.sign(
    "HMAC",
    key,
    new TextEncoder().encode(signingInput),
  );

  return `${signingInput}.${base64UrlEncode(new Uint8Array(sig))}`;
}
