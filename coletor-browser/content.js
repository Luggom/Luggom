// Luggom Coletor - content script.
// Injeta um "espiao" no contexto da PAGINA (o banco bloqueia DevTools; o CSP
// da pagina permite script inline). O espiao apenas OBSERVA: registra as
// chamadas de criptografia (assinatura ECDSA + chave efemera) e as
// requisicoes/respostas XHR do app do banco. O content script repassa isso ao
// background, que envia ao coletor local (127.0.0.1:8099).

const HOOK = `(() => {
  if (window.__lgHooked) return;
  window.__lgHooked = true;
  const b64 = (x) => btoa(String.fromCharCode(...new Uint8Array(x)));
  const L = (window.__lg = { v: 3, signs: [], reqs: [], resps: [], cryptos: [] });
  let t = null;
  const push = () => {
    if (t) return;
    t = setTimeout(() => {
      t = null;
      try { window.postMessage({ __luggom_push: JSON.stringify(window.__lg) }, '*'); } catch (e) {}
    }, 1500);
  };
  const keySpki = new Map();
  const SPKI_P256_PREFIX = '3059301306072a8648ce3d020106082a8648ce3d03010703420004';
  const hex = (u8) => Array.from(u8).map((x) => x.toString(16).padStart(2, '0')).join('');
  const b64u = (s) => {
    s = String(s).replace(/-/g, '+').replace(/_/g, '/');
    while (s.length % 4) s += '=';
    const bin = atob(s);
    const u = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) u[i] = bin.charCodeAt(i);
    return u;
  };
  const spkiFromXY = (x, y) => {
    try {
      const raw = b64u(SPKI_P256_PREFIX + hex(b64u(x)) + hex(b64u(y)));
      return b64(raw);
    } catch (e) { return null; }
  };
  const S = crypto.subtle.sign.bind(crypto.subtle);
  const E = crypto.subtle.exportKey.bind(crypto.subtle);
  const G = crypto.subtle.generateKey.bind(crypto.subtle);
  const I = crypto.subtle.importKey.bind(crypto.subtle);
  crypto.subtle.sign = async (a, k, d) => {
    let spki = keySpki.get(k) || null;
    if (!spki) {
      try { if (k && k.extractable) spki = b64(await E('spki', k)); } catch (e) {}
    }
    const s = await S(a, k, d);
    try {
      L.signs.push({
        canonical: new TextDecoder().decode(d),
        signature: b64(s),
        spki: spki
      });
      push();
    } catch (e) {}
    return s;
  };
  crypto.subtle.generateKey = async (...args) => {
    const o = await G(...args);
    try {
      if (o && o.publicKey && o.privateKey) {
        let sp = null;
        try { sp = b64(await E('spki', o.publicKey)); } catch (e) {}
        if (sp) { keySpki.set(o.privateKey, sp); keySpki.set(o.publicKey, sp); }
      }
    } catch (e) {}
    return o;
  };
  crypto.subtle.importKey = async (f, kd, ...rest) => {
    const o = await I(f, kd, ...rest);
    try {
      if (o) {
        if (f === 'jwk' && kd && kd.x && kd.y) {
          const sp = spkiFromXY(kd.x, kd.y);
          if (sp) keySpki.set(o, sp);
        } else if (f === 'spki') {
          keySpki.set(o, b64(kd));
        }
      }
    } catch (e) {}
    return o;
  };
  const E2 = crypto.subtle.exportKey.bind(crypto.subtle);
  crypto.subtle.exportKey = async (f, k) => {
    const o = await E2(f, k);
    try {
      if (f === 'spki') keySpki.set(k, b64(o));
    } catch (e) {}
    return o;
  };
  const toU8 = (x) => {
    if (!x) return new Uint8Array(0);
    if (x instanceof ArrayBuffer) return new Uint8Array(x);
    if (ArrayBuffer.isView(x)) return new Uint8Array(x.buffer, x.byteOffset, x.byteLength);
    return new Uint8Array(0);
  };
  const algInfo = (a) => ({
    name: a && a.name,
    hash: a && a.hash && a.hash.name,
    label: a && a.label ? b64(toU8(a.label)) : null
  });
  const EN = crypto.subtle.encrypt.bind(crypto.subtle);
  crypto.subtle.encrypt = async (a, k, d) => {
    const c = await EN(a, k, d);
    try {
      L.cryptos.push({ op: 'encrypt', alg: algInfo(a), plaintext: b64(toU8(d)), ciphertext: b64(toU8(c)) });
      push();
    } catch (e) {}
    return c;
  };
  const DE = crypto.subtle.decrypt.bind(crypto.subtle);
  crypto.subtle.decrypt = async (a, k, d) => {
    const p = await DE(a, k, d);
    try {
      L.cryptos.push({ op: 'decrypt', alg: algInfo(a), plaintext: b64(toU8(p)), ciphertext: b64(toU8(d)) });
      push();
    } catch (e) {}
    return p;
  };
  // Instantaneo do estado de cifra: QUAL chave RSA o app tem em uso
  // (window.encryptionLib.publicKey) + sessionStorage. Resolve de vez a duvida
  // "x_response ou PUBLIC_RSA do JWT?" sem tocar no banco.
  const snap = () => {
    const o = { href: '', encKey: null, pk: null, token: null, cookie: null, ss: {} };
    try { o.href = location.href; } catch (e) {}
    try { o.encKey = (window.encryptionLib && window.encryptionLib.publicKey) || null; } catch (e) {}
    try { o.pk = sessionStorage.getItem('pk'); } catch (e) {}
    try { o.token = sessionStorage.getItem('token'); } catch (e) {}
    // nenhum cookie da Viacredi e httpOnly -> document.cookie traz a sessao toda
    try { o.cookie = document.cookie || null; } catch (e) {}
    try {
      for (const k of ['clientId', 'accessId']) {
        const v = sessionStorage.getItem(k);
        if (v) o.ss[k] = v.length > 500 ? v.slice(0, 500) : v;
      }
    } catch (e) {}
    return o;
  };
  const OP = XMLHttpRequest.prototype.open;
  const SH = XMLHttpRequest.prototype.setRequestHeader;
  const SE = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function (m, u) {
    this.__r = { method: m, url: u, headers: {} };
    return OP.apply(this, arguments);
  };
  XMLHttpRequest.prototype.setRequestHeader = function (n, v) {
    if (this.__r) {
      this.__r.headers[n] = v;
      // no instante exato em que o X-Request e calculado, a chave em uso e esta
      if (String(n).toLowerCase() === 'x-request') this.__r.encKeyAtRequest = snap();
    }
    return SH.apply(this, arguments);
  };
  XMLHttpRequest.prototype.send = function (body) {
    if (this.__r) {
      this.__r.body = typeof body === 'string' ? body.slice(0, 8000) : null;
      L.reqs.push(this.__r);
      push();
    }
    this.addEventListener('load', () => {
      try {
        L.resps.push({
          url: this.responseURL,
          status: this.status,
          body: (this.responseText || '').slice(0, 40000)
        });
        push();
      } catch (e) {}
    });
    return SE.apply(this, arguments);
  };
  // fetch() — bancos modernos (possivelmente a Caixa) usam fetch em vez de XHR.
  const F = window.fetch;
  if (F) {
    window.fetch = function (input, init) {
      try {
        const isReq = input && typeof input === 'object' && 'url' in input;
        const r = {
          method: (init && init.method) || (isReq && input.method) || 'GET',
          url: isReq ? input.url : String(input),
          headers: {}
        };
        try {
          const h = (init && init.headers) || (isReq && input.headers);
          if (h) {
            if (Array.isArray(h)) h.forEach((kv) => { r.headers[kv[0]] = String(kv[1]).slice(0, 2000); });
            else if (typeof h.forEach === 'function') h.forEach((v, k) => { r.headers[k] = String(v).slice(0, 2000); });
            else Object.keys(h).forEach((k) => { r.headers[k] = String(h[k]).slice(0, 2000); });
          }
        } catch (e) {}
        try { if (init && typeof init.body === 'string') r.body = init.body.slice(0, 8000); } catch (e) {}
        L.reqs.push(r);
        push();
      } catch (e) {}
      const resp = F.apply(this, arguments);
      try {
        Promise.resolve(resp).then((res) => {
          try {
            res.clone().text().then((t) => {
              try { L.resps.push({ url: res.url, status: res.status, body: (t || '').slice(0, 40000) }); push(); } catch (e) {}
            }).catch(() => {});
          } catch (e) {}
        }).catch(() => {});
      } catch (e) {}
      return resp;
    };
  }
  window.addEventListener('message', (e) => {
    if (e.source === window && e.data === 'luggom-get') {
      window.postMessage({ __luggom_pull: JSON.stringify(window.__lg) }, '*');
    }
  });
  L.installed = Date.now();
  push();
  console.log('Luggom hook instalado');
})();`;

function injectHook() {
  const el = document.createElement('script');
  el.textContent = HOOK;
  (document.head || document.documentElement).appendChild(el);
  el.remove();
}

injectHook();

// Repassa automaticamente cada captura ao background (que faz o POST local).
window.addEventListener('message', (e) => {
  if (e.source === window && e.data && e.data.__luggom_push) {
    browser.runtime.sendMessage({ type: 'luggom-data', data: e.data.__luggom_push }).catch(() => {});
  }
});

// §13.137 — REPLAY dentro do TAB (same-origin, mundo isolado da extensão: o JS
// da PÁGINA não participa). Pergunta que isola: o WAF da Caixa exige o
// fingerprint do Firefox (⇒ passa) ou o JS/token da página (⇒ 302)?
// Feito aqui, e não na background, porque o tab está vivo e same-origin (a
// background abortava o fetch).
browser.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || msg.type !== 'luggom-replay') return undefined;

  (async () => {
    const req = msg.req || {};
    const out = {
      onde: 'content-script',
      pedido: (req.url || '').split('?')[0],
      metodo: (req.method || 'POST').toUpperCase(),
      bodyLen: (req.body || '').length
    };
    try {
      const headers = {};
      for (const [k, v] of Object.entries(req.headers || {})) headers[k] = v;
      const r = await fetch(req.url, {
        method: out.metodo,
        headers,
        body: req.body || undefined,
        credentials: 'include',
        redirect: 'follow'
      });
      const t = await r.text();
      out.status = r.status;
      out.final = (r.url || '').split('?')[0];
      out.sessaoEncerrada = (r.url || '').includes('sessaoEncerrada');
      out.bytes = t.length;
      // Prova da FRESCURA da ligação: no Resource Timing, uma ligação REUTILIZADA
      // tem connectStart/connectEnd a 0; uma ligação NOVA abre TCP+TLS e tem
      // valores > 0. É o que distingue "o WAF aceita um cliente novo" de
      // "reusámos a ligação já aceite" (§13.137-A).
      try {
        const ent = performance.getEntriesByName(req.url).pop();
        out.timing = ent
          ? {
              proto: ent.nextHopProtocol,
              connectStart: Math.round(ent.connectStart),
              connectEnd: Math.round(ent.connectEnd),
              duration: Math.round(ent.duration)
            }
          : 'sem entrada em performance';
      } catch (e2) {
        out.timing = String(e2);
      }
    } catch (e) {
      out.error = String(e);
    }
    sendResponse(out);
  })();

  return true;
});

// Fallback manual (clique no icone).
browser.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || msg.type !== 'luggom-collect') return undefined;

  const onMsg = (e) => {
    if (e.source === window && e.data && e.data.__luggom_pull) {
      window.removeEventListener('message', onMsg);
      sendResponse({ data: e.data.__luggom_pull });
    }
  };
  window.addEventListener('message', onMsg);
  window.postMessage('luggom-get', '*');

  setTimeout(() => {
    window.removeEventListener('message', onMsg);
    sendResponse({ error: 'sem dados' });
  }, 4000);

  return true;
});
