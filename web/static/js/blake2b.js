// Minimal Blake2b for Beacon sealed-box nonces (24-byte digest, no key).
// Compatible with golang.org/x/crypto/blake2b.New(24, nil).
export function blake2b(input, outlen) {
  outlen = outlen || 64;
  const IV = new Uint32Array([
    0xf3bcc908, 0x6a09e667, 0x84caa73b, 0xbb67ae85,
    0xfe94f82b, 0x3c6ef372, 0x5f1d36f1, 0xa54ff53a,
    0xade682d1, 0x510e527f, 0x2b3e6c1f, 0x9b05688c,
    0xfb41bd6b, 0x1f83d9ab, 0x137e2179, 0x5be0cd19,
  ]);
  const SIGMA = [
    [0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15],
    [14,10,4,8,9,15,13,6,1,12,0,2,11,7,5,3],
    [11,8,12,0,5,2,15,13,10,14,3,6,7,1,9,4],
    [7,9,3,1,13,12,11,14,2,6,5,10,4,0,15,8],
    [9,0,5,7,2,4,10,15,14,1,11,12,6,8,3,13],
    [2,12,6,10,0,11,8,3,4,13,7,5,15,14,1,9],
    [12,5,1,15,14,13,4,10,0,7,6,3,9,2,8,11],
    [13,11,7,14,12,1,3,9,5,0,15,4,8,6,2,10],
    [6,15,14,9,11,3,0,8,12,2,13,7,1,4,10,5],
    [10,2,8,4,7,6,1,5,15,11,9,14,3,12,13,0],
    [0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15],
    [14,10,4,8,9,15,13,6,1,12,0,2,11,7,5,3],
  ];

  function add64(a0, a1, b0, b1) {
    const lo = (a0 >>> 0) + (b0 >>> 0);
    return [lo >>> 0, (((a1 >>> 0) + (b1 >>> 0) + ((lo / 0x100000000) | 0)) >>> 0)];
  }
  function xor64(a0, a1, b0, b1) { return [(a0 ^ b0) >>> 0, (a1 ^ b1) >>> 0]; }
  function rotr64(a0, a1, n) {
    if (n === 32) return [a1, a0];
    if (n < 32) {
      return [
        ((a0 >>> n) | (a1 << (32 - n))) >>> 0,
        ((a1 >>> n) | (a0 << (32 - n))) >>> 0,
      ];
    }
    n -= 32;
    return [
      ((a1 >>> n) | (a0 << (32 - n))) >>> 0,
      ((a0 >>> n) | (a1 << (32 - n))) >>> 0,
    ];
  }

  const h = new Uint32Array(16);
  h.set(IV);
  h[0] ^= 0x01010000 ^ outlen;
  let t0 = 0, t1 = 0;
  let last = false;
  const buf = new Uint8Array(128);
  let buflen = 0;

  function compress(block, isLast) {
    const m = new Uint32Array(32);
    for (let i = 0; i < 32; i++) {
      const j = i * 4;
      m[i] = block[j] | (block[j + 1] << 8) | (block[j + 2] << 16) | (block[j + 3] << 24);
    }
    const v = new Uint32Array(32);
    for (let i = 0; i < 16; i++) v[i] = h[i];
    for (let i = 0; i < 16; i++) v[i + 16] = IV[i];
    v[24] ^= t0; v[25] ^= t1;
    if (isLast) { v[28] = ~v[28]; v[29] = ~v[29]; }

    function G(a, b, c, d, x0, x1, y0, y1) {
      let p;
      p = add64(v[2*a], v[2*a+1], v[2*b], v[2*b+1]);
      p = add64(p[0], p[1], x0, x1);
      v[2*a] = p[0]; v[2*a+1] = p[1];
      p = xor64(v[2*d], v[2*d+1], v[2*a], v[2*a+1]);
      p = rotr64(p[0], p[1], 32);
      v[2*d] = p[0]; v[2*d+1] = p[1];
      p = add64(v[2*c], v[2*c+1], v[2*d], v[2*d+1]);
      v[2*c] = p[0]; v[2*c+1] = p[1];
      p = xor64(v[2*b], v[2*b+1], v[2*c], v[2*c+1]);
      p = rotr64(p[0], p[1], 24);
      v[2*b] = p[0]; v[2*b+1] = p[1];
      p = add64(v[2*a], v[2*a+1], v[2*b], v[2*b+1]);
      p = add64(p[0], p[1], y0, y1);
      v[2*a] = p[0]; v[2*a+1] = p[1];
      p = xor64(v[2*d], v[2*d+1], v[2*a], v[2*a+1]);
      p = rotr64(p[0], p[1], 16);
      v[2*d] = p[0]; v[2*d+1] = p[1];
      p = add64(v[2*c], v[2*c+1], v[2*d], v[2*d+1]);
      v[2*c] = p[0]; v[2*c+1] = p[1];
      p = xor64(v[2*b], v[2*b+1], v[2*c], v[2*c+1]);
      p = rotr64(p[0], p[1], 63);
      v[2*b] = p[0]; v[2*b+1] = p[1];
    }

    for (let r = 0; r < 12; r++) {
      const s = SIGMA[r];
      G(0,4,8,12, m[2*s[0]], m[2*s[0]+1], m[2*s[1]], m[2*s[1]+1]);
      G(1,5,9,13, m[2*s[2]], m[2*s[2]+1], m[2*s[3]], m[2*s[3]+1]);
      G(2,6,10,14, m[2*s[4]], m[2*s[4]+1], m[2*s[5]], m[2*s[5]+1]);
      G(3,7,11,15, m[2*s[6]], m[2*s[6]+1], m[2*s[7]], m[2*s[7]+1]);
      G(0,5,10,15, m[2*s[8]], m[2*s[8]+1], m[2*s[9]], m[2*s[9]+1]);
      G(1,6,11,12, m[2*s[10]], m[2*s[10]+1], m[2*s[11]], m[2*s[11]+1]);
      G(2,7,8,13, m[2*s[12]], m[2*s[12]+1], m[2*s[13]], m[2*s[13]+1]);
      G(3,4,9,14, m[2*s[14]], m[2*s[14]+1], m[2*s[15]], m[2*s[15]+1]);
    }
    for (let i = 0; i < 16; i++) h[i] ^= v[i] ^ v[i + 16];
  }

  function update(data) {
    data = data instanceof Uint8Array ? data : new Uint8Array(data);
    let offset = 0;
    while (offset < data.length) {
      if (buflen === 128) {
        t0 = (t0 + 128) >>> 0;
        if (t0 < 128) t1 = (t1 + 1) >>> 0;
        compress(buf, false);
        buflen = 0;
      }
      const take = Math.min(128 - buflen, data.length - offset);
      buf.set(data.subarray(offset, offset + take), buflen);
      buflen += take;
      offset += take;
    }
  }

  function digest() {
    if (last) throw new Error('blake2b already finalized');
    last = true;
    t0 = (t0 + buflen) >>> 0;
    if (t0 < buflen) t1 = (t1 + 1) >>> 0;
    while (buflen < 128) buf[buflen++] = 0;
    compress(buf, true);
    const out = new Uint8Array(outlen);
    const tmp = new Uint8Array(64);
    for (let i = 0; i < 16; i++) {
      tmp[i * 4] = h[i] & 0xff;
      tmp[i * 4 + 1] = (h[i] >>> 8) & 0xff;
      tmp[i * 4 + 2] = (h[i] >>> 16) & 0xff;
      tmp[i * 4 + 3] = (h[i] >>> 24) & 0xff;
    }
    out.set(tmp.subarray(0, outlen));
    return out;
  }

  update(input);
  return digest();
}
