// Luggom Coletor - background.
// Recebe do content script (automatico a cada captura) e envia ao coletor
// local. O clique no icone forca um envio manual (fallback).

const COLETOR = 'http://127.0.0.1:8099/collect';

// Cookies httpOnly: o content script só vê `document.cookie`, que os OMITE —
// era por isso que as capturas da Caixa vinham com zero cookies (o cookie de
// sessão da Caixa é httpOnly). Com a permissão `cookies` + host_permissions do
// domínio do banco, o background lê o cookie jar do browser, que os INCLUI.
async function withCookies(dataString, tabUrl) {
  try {
    if (!tabUrl) return dataString;
    const jar = await browser.cookies.getAll({ url: tabUrl });
    if (!jar || !jar.length) return dataString;
    const obj = JSON.parse(dataString);
    obj.cookies = jar.map((c) => c.name + '=' + c.value).join('; ');
    // Origem da captura: o `reqs[].url` do content script é um PATH relativo —
    // sem isto não se sabe contra que host repetir o pedido (verify_rail.py).
    if (tabUrl) obj.pageUrl = tabUrl;
    // Diagnóstico sem valores: quais eram httpOnly (os que faltavam antes).
    obj.cookiesDetail = jar.map((c) => ({
      name: c.name,
      httpOnly: !!c.httpOnly,
      secure: !!c.secure,
      domain: c.domain
    }));
    return JSON.stringify(obj);
  } catch (e) {
    return dataString;
  }
}

async function send(data) {
  try {
    const res = await fetch(COLETOR, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: data
    });
    return res.ok;
  } catch (e) {
    return false;
  }
}

function badge(text, tabId) {
  try {
    browser.action.setBadgeText({ text, tabId });
    browser.action.setBadgeBackgroundColor({ color: text === 'OK' ? '#2e7d32' : '#c62828', tabId });
  } catch (e) {}
}

// REPLAY com o PRÓPRIO Firefox do utilizador (154) — §13.137.
//
// Pergunta que isto isola: o WAF da Caixa exige o *fingerprint* do Firefox
// (⇒ bastaria uma ligação TLS nova e isto passaria) ou exige o *JS/contexto da
// página* (⇒ isto falha, mesmo sendo Firefox, e só um browser a sério serve)?
//
// É o mesmo Firefox, a mesma versão, a mesma máquina e o MESMO jar de cookies
// (a captura traz 22, com os __uzm*), mas numa ligação TLS NOVA e fora do
// contexto JS da página. O curl_cffi (que impersona o Firefox mas não é o
// Firefox) já deu 302 em todas as versões — ver §13.137.
//
// Limitação honesta: o fetch sai do content script (mundo isolado da extensão,
// same-origin no tab) — o JS da PÁGINA não participa, mas o `Origin`/`Referer` e
// os `Sec-Fetch-*` são os do Firefox a pedir de dentro da página. É o mais perto
// de "Firefox sem o JS do banco" que se consegue sem outro browser.
async function firefoxReplay(tabId, obj) {
  try {
    const reqs = (obj && obj.reqs) || [];
    // Prioridade: o extrato (é o pedido que o browser deu 200 e que o curl_cffi já
    // testou); se não existir, o comprovante, depois o detalhe.
    const rotas = ['consulta-extratos', 'comprovantePixExtratoSucesso', 'detalheExtrato'];
    let alvo = null;
    for (const rota of rotas) {
      for (const q of reqs) {
        if ((q.url || '').includes(rota)) alvo = q;
      }
      if (alvo) break;
    }
    if (!alvo) return { skipped: 'sem rota conhecida na captura' };
    // Delegado ao content script: o tab está vivo e é same-origin. Na background
    // o fetch era abortado (event page suspenso) e nunca chegámos a ver o status.
    return await browser.tabs.sendMessage(tabId, { type: 'luggom-replay', req: alvo });
  } catch (e) {
    return { error: String(e) };
  }
}

browser.runtime.onMessage.addListener((msg, sender) => {
  if (msg && msg.type === 'luggom-data' && typeof msg.data === 'string') {
    const tabId = sender && sender.tab ? sender.tab.id : undefined;
    const tabUrl = sender && sender.tab ? sender.tab.url : undefined;
    withCookies(msg.data, tabUrl).then((data) => send(data).then((ok) => badge(ok ? 'OK' : 'ERR', tabId)));
  }
  return undefined;
});

browser.action.onClicked.addListener(async (tab) => {
  badge('...', tab.id);
  try {
    const r = await browser.tabs.sendMessage(tab.id, { type: 'luggom-collect' });
    if (!r || !r.data) {
      badge('VAZIO', tab.id);
      return;
    }
    const data = await withCookies(r.data, tab.url);
    // §13.137: replay com o PRÓPRIO Firefox (mesmo jar, ligação nova, sem o JS da
    // página) — separa "fingerprint do Firefox" de "JS/contexto da página".
    try {
      const obj = JSON.parse(data);
      obj.firefoxReplay = await firefoxReplay(tab.id, obj);
      badge((await send(JSON.stringify(obj))) ? 'OK' : 'ERR', tab.id);
      return;
    } catch (e) {}
    badge((await send(data)) ? 'OK' : 'ERR', tab.id);
  } catch (e) {
    badge('FALHA', tab.id);
  }
});
