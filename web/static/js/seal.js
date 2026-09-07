// Client-side sealed-box encrypt matching golang.org/x/crypto/nacl/box.SealAnonymous.
// Depends on global `nacl` from vendor/nacl-fast.min.js and blake2b.js.

import { blake2b } from "./blake2b.js";

function b64ToBytes(b64) {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function bytesToB64(bytes) {
  let s = "";
  for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]);
  return btoa(s);
}

function sealNonce(ephemeralPub, recipientPub) {
  const msg = new Uint8Array(64);
  msg.set(ephemeralPub, 0);
  msg.set(recipientPub, 32);
  return blake2b(msg, 24);
}

/**
 * Seal message to recipient public key (32 bytes).
 * Returns ciphertext: ephemeral_pk (32) || nacl.box(...)
 */
export function sealAnonymous(message, recipientPublicKey) {
  const nacl = globalThis.nacl;
  if (!nacl || !nacl.box) {
    throw new Error("nacl is not loaded");
  }
  const recipient =
    recipientPublicKey instanceof Uint8Array
      ? recipientPublicKey
      : b64ToBytes(recipientPublicKey);
  if (recipient.length !== 32) {
    throw new Error("recipient public key must be 32 bytes");
  }
  const msg =
    typeof message === "string"
      ? new TextEncoder().encode(message)
      : message instanceof Uint8Array
        ? message
        : new Uint8Array(message);

  const ephemeral = nacl.box.keyPair();
  const nonce = sealNonce(ephemeral.publicKey, recipient);
  const boxed = nacl.box(msg, nonce, recipient, ephemeral.secretKey);
  if (!boxed) {
    throw new Error("nacl.box failed");
  }
  const out = new Uint8Array(32 + boxed.length);
  out.set(ephemeral.publicKey, 0);
  out.set(boxed, 32);
  return out;
}

export function sealIdentityPayload(payload, recipientPublicKeyB64) {
  const json = JSON.stringify(payload);
  return bytesToB64(sealAnonymous(json, recipientPublicKeyB64));
}

export { b64ToBytes, bytesToB64 };
