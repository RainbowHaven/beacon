function b64urlToBuf(value) {
  const pad = '='.repeat((4 - (value.length % 4)) % 4);
  const str = (value + pad).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(str);
  const buf = new ArrayBuffer(raw.length);
  const view = new Uint8Array(buf);
  for (let i = 0; i < raw.length; i++) view[i] = raw.charCodeAt(i);
  return buf;
}

function bufToB64url(buf) {
  const view = new Uint8Array(buf);
  let str = '';
  for (let i = 0; i < view.length; i++) str += String.fromCharCode(view[i]);
  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function reviveCreationOptions(options) {
  options.challenge = b64urlToBuf(options.challenge);
  options.user.id = b64urlToBuf(options.user.id);
  if (options.excludeCredentials) {
    options.excludeCredentials = options.excludeCredentials.map((c) => ({
      ...c,
      id: b64urlToBuf(c.id),
    }));
  }
  return options;
}

function reviveRequestOptions(options) {
  options.challenge = b64urlToBuf(options.challenge);
  if (options.allowCredentials) {
    options.allowCredentials = options.allowCredentials.map((c) => ({
      ...c,
      id: b64urlToBuf(c.id),
    }));
  }
  return options;
}

function publicKeyCredentialToJSON(cred) {
  const clientExtensionResults = {};
  if (cred.getClientExtensionResults) {
    Object.assign(clientExtensionResults, cred.getClientExtensionResults());
  }
  return {
    id: cred.id,
    rawId: bufToB64url(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment,
    response: {
      clientDataJSON: bufToB64url(cred.response.clientDataJSON),
      attestationObject: cred.response.attestationObject
        ? bufToB64url(cred.response.attestationObject)
        : undefined,
      authenticatorData: cred.response.authenticatorData
        ? bufToB64url(cred.response.authenticatorData)
        : undefined,
      signature: cred.response.signature
        ? bufToB64url(cred.response.signature)
        : undefined,
      userHandle: cred.response.userHandle
        ? bufToB64url(cred.response.userHandle)
        : undefined,
      transports: cred.response.getTransports
        ? cred.response.getTransports()
        : undefined,
    },
    clientExtensionResults,
  };
}

async function postJSON(url, body) {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  const ct = res.headers.get('content-type') || '';
  if (ct.includes('application/json')) return res.json();
  return null;
}

export async function enrollPasskey(token) {
  const qs = '?token=' + encodeURIComponent(token);
  const creation = await postJSON('/webauthn/register/begin' + qs, { token });
  const publicKey = reviveCreationOptions(creation.publicKey);
  const cred = await navigator.credentials.create({ publicKey });
  const payload = publicKeyCredentialToJSON(cred);
  const res = await fetch('/webauthn/register/finish' + qs, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function addPasskey() {
  const creation = await postJSON('/account/passkeys/begin');
  const publicKey = reviveCreationOptions(creation.publicKey);
  const cred = await navigator.credentials.create({ publicKey });
  const payload = publicKeyCredentialToJSON(cred);
  const res = await fetch('/account/passkeys/finish', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function loginWithPasskey(email) {
  const assertion = await postJSON('/webauthn/login/begin', { email });
  const publicKey = reviveRequestOptions(assertion.publicKey);
  const cred = await navigator.credentials.get({ publicKey });
  const payload = publicKeyCredentialToJSON(cred);
  const res = await fetch('/webauthn/login/finish', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}
