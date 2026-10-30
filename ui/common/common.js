// PoseaVPN — shared frontend helpers
(function () {
  const api = () => window.go.guiapp.App;

  const icons = {
    shield: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l8 3v6c0 4.5-3.2 8.3-8 9-4.8-.7-8-4.5-8-9V6l8-3z"/><path d="M9 12l2 2 4-4"/></svg>',
    power: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v9"/><path d="M6.3 6.3a8 8 0 1 0 11.4 0"/></svg>',
    plus: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>',
    edit: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/></svg>',
    file: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/></svg>',
    folder: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>',
    trash: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 7h16M10 11v6M14 11v6M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12M9 7V4h6v3"/></svg>',
    down: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14M6 13l6 6 6-6"/></svg>',
    up: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 19V5M6 11l6-6 6 6"/></svg>',
    export: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>',
    copy: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>',
    refresh: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>',
    key: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="7.5" cy="15.5" r="5.5"/><path d="M11.4 11.6L21 2M15.5 7.5L18 10M18.5 4.5L21 7"/></svg>',
  };

  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  function fmtBytes(n) {
    n = Number(n) || 0;
    const u = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return (i === 0 ? n.toFixed(0) : n.toFixed(n < 10 ? 2 : 1)) + ' ' + u[i];
  }

  function fmtAgo(unix) {
    if (!unix) return 'henüz yok';
    const s = Math.max(0, Math.floor(Date.now() / 1000 - unix));
    if (s < 5) return 'şimdi';
    if (s < 60) return s + ' sn önce';
    if (s < 3600) return Math.floor(s / 60) + ' dk önce';
    if (s < 86400) return Math.floor(s / 3600) + ' sa önce';
    return Math.floor(s / 86400) + ' gün önce';
  }

  function fmtUptime(unix) {
    if (!unix) return '—';
    let s = Math.max(0, Math.floor(Date.now() / 1000 - unix));
    const h = Math.floor(s / 3600); s -= h * 3600;
    const m = Math.floor(s / 60); s -= m * 60;
    const p = (x) => String(x).padStart(2, '0');
    return `${p(h)}:${p(m)}:${p(s)}`;
  }

  function toast(msg, type = '') {
    let box = document.getElementById('toasts');
    if (!box) { box = document.createElement('div'); box.id = 'toasts'; document.body.appendChild(box); }
    const el = document.createElement('div');
    el.className = 'toast ' + type;
    el.textContent = msg;
    box.appendChild(el);
    setTimeout(() => { el.style.opacity = '0'; el.style.transition = 'opacity .3s'; setTimeout(() => el.remove(), 300); }, type === 'bad' ? 7000 : 3800);
  }

  // Wails rejects with a string or Error — normalise it.
  const errText = (e) => (e && e.message) ? e.message : String(e);

  async function guard(fn, okMsg) {
    try {
      const r = await fn();
      if (okMsg) toast(okMsg, 'ok');
      return r;
    } catch (e) {
      toast(errText(e), 'bad');
      return undefined;
    }
  }

  function modal({ title, body, actions, wide, sticky, primary, cancel, focus }) {
    return new Promise((resolve) => {
      const bg = document.createElement('div');
      bg.className = 'modal-bg';
      bg.innerHTML = `<div class="modal ${wide ? 'wide' : ''}"><h2>${esc(title)}</h2><div class="mbody"></div><div class="row"></div></div>`;
      const mb = bg.querySelector('.mbody');
      if (typeof body === 'string') mb.innerHTML = body; else mb.appendChild(body);
      const row = bg.querySelector('.row');
      const close = (v) => { bg.remove(); resolve(v); };

      let acts = actions;
      if (!acts || !Array.isArray(acts)) {
        acts = [];
        if (cancel !== false) {
          acts.push({ label: cancel || 'İptal', value: false });
        }
        acts.push({ label: primary || 'Tamam', cls: 'primary', value: true });
      }

      acts.forEach((a) => {
        const b = document.createElement('button');
        b.className = 'btn ' + (a.cls || '');
        b.textContent = a.label;
        b.onclick = () => close(a.value);
        row.appendChild(b);
      });

      bg.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && e.target && e.target.tagName === 'INPUT') {
          const prim = acts.find((x) => x.cls && x.cls.includes('primary'));
          if (prim) {
            e.preventDefault();
            close(prim.value);
          }
        } else if (e.key === 'Escape') {
          close(null);
        }
      });

      // a sticky modal (editor) ignores clicks on the backdrop so edits are not lost by accident
      if (!sticky) bg.addEventListener('mousedown', (e) => { if (e.target === bg) close(null); });
      document.body.appendChild(bg);
      if (focus && focus.focus) {
        setTimeout(() => focus.focus(), 50);
      } else {
        const first = bg.querySelector('input,textarea'); if (first) setTimeout(() => first.focus(), 50);
      }
      bg._close = close;
    });
  }

  const confirmBox = (title, text, okLabel = 'Evet') =>
    modal({ title, body: `<p>${esc(text)}</p>`, actions: [{ label: 'Vazgeç', value: false }, { label: okLabel, cls: 'primary', value: true }] });

  // Service status pill text
  function serviceInfo(s) {
    switch (s) {
      case 'running': return { cls: 'ok', text: 'Servis çalışıyor' };
      case 'stopped': return { cls: 'warn', text: 'Servis durdu' };
      case 'notinstalled': return { cls: 'bad', text: 'Servis kurulu değil' };
      default: return { cls: '', text: 'Servis durumu bilinmiyor' };
    }
  }

  // Banner shown when the service is not running (install / start button).
  function renderBanner(host, state, onAction) {
    // rebuild only when the service state changes (otherwise the entrance animation replays on every poll)
    if (host.dataset.sig === state.service) return;
    host.dataset.sig = state.service;
    if (state.service === 'running') { host.innerHTML = ''; return; }
    const notInstalled = state.service === 'notinstalled';
    host.innerHTML = `
      <div class="banner ${notInstalled ? 'bad' : ''} fade">
        <div class="txt">
          <b>${notInstalled ? 'Arka plan servisi kurulu değil' : 'Arka plan servisi çalışmıyor'}</b>
          <span>Tünel açabilmek için servisin çalışması gerekir. Tek seferlik yönetici onayı istenir.</span>
        </div>
        <button class="btn primary" id="svcBtn">${notInstalled ? 'Servisi kur ve başlat' : 'Servisi başlat'}</button>
      </div>`;
    host.querySelector('#svcBtn').onclick = async () => {
      const fn = notInstalled ? api().ServiceInstall : api().ServiceStart;
      const ok = await guard(() => fn(), 'Yönetici onayı bekleniyor…');
      if (ok !== undefined && onAction) onAction();
    };
  }

  async function copyToClipboard(text) {
    if (!text) return false;
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text);
      } else if (api().SetClipboard) {
        await api().SetClipboard(text);
      }
      toast('Açık anahtar panoya kopyalandı', 'ok');
      return true;
    } catch (e) {
      try {
        if (api().SetClipboard) {
          await api().SetClipboard(text);
          toast('Açık anahtar panoya kopyalandı', 'ok');
          return true;
        }
      } catch (err) {}
      toast('Kopyalanamadı: ' + errText(e), 'bad');
      return false;
    }
  }

  async function openCreateTunnelModal(onSuccess) {
    let kp = { privateKey: '', publicKey: '' };
    try {
      kp = await api().GenerateKeyPair();
    } catch (e) {
      toast('Anahtar üretilemedi: ' + errText(e), 'bad');
      return;
    }

    const wrap = document.createElement('div');
    wrap.innerHTML = `
      <div class="field">
        <label>Tünel adı</label>
        <input class="input" id="ctName" placeholder="ör. ofis-vpn" value="" autocomplete="off" />
      </div>

      <div class="field" style="margin-top:12px">
        <label style="display:flex;justify-content:space-between;align-items:center">
          <span>Açık anahtar (Public Key) — <span style="color:var(--accent2);font-weight:500">Sunucu yöneticisine verilecek anahtar</span></span>
        </label>
        <div style="display:flex;gap:8px;align-items:center;margin-top:4px">
          <input class="input mono" id="ctPubKey" readonly style="flex:1;background:rgba(0,0,0,.35);cursor:text;color:var(--accent2);font-weight:600" value="${esc(kp.publicKey)}" />
          <button class="btn sm" id="ctCopyBtn" type="button" title="Panoya kopyala">${icons.copy} Kopyala</button>
          <button class="btn sm" id="ctRegenBtn" type="button" title="Yeni bir anahtar çifti üret">${icons.refresh} Yeni Anahtar</button>
        </div>
      </div>

      <div class="field" style="margin-top:12px">
        <label>Yapılandırma (.conf içeriği)</label>
        <textarea class="textarea tall" id="ctText" spellcheck="false"></textarea>
        <div class="validity" id="ctValid"></div>
      </div>
    `;

    const nameEl = wrap.querySelector('#ctName');
    const pubEl = wrap.querySelector('#ctPubKey');
    const textEl = wrap.querySelector('#ctText');
    const validEl = wrap.querySelector('#ctValid');
    const copyBtn = wrap.querySelector('#ctCopyBtn');
    const regenBtn = wrap.querySelector('#ctRegenBtn');

    const templateFor = (priv) => `[Interface]
PrivateKey = ${priv}
Address = 10.0.0.2/24
DNS = 1.1.1.1

[Peer]
PublicKey = 
AllowedIPs = 0.0.0.0/0
Endpoint = 
`;

    textEl.value = templateFor(kp.privateKey);

    copyBtn.onclick = () => copyToClipboard(pubEl.value);

    regenBtn.onclick = async () => {
      try {
        const newKp = await api().GenerateKeyPair();
        kp = newKp;
        pubEl.value = newKp.publicKey;
        pubEl.style.color = 'var(--accent2)';
        const cur = textEl.value;
        const re = /(PrivateKey\s*=\s*)[^\r\n]+/i;
        if (re.test(cur)) {
          textEl.value = cur.replace(re, `$1${newKp.privateKey}`);
        } else {
          textEl.value = templateFor(newKp.privateKey);
        }
        await check();
        toast('Yeni anahtar çifti oluşturuldu', 'ok');
      } catch (err) {
        toast('Anahtar üretilemedi: ' + errText(err), 'bad');
      }
    };

    let timer = null;
    const check = async () => {
      const val = textEl.value;
      const m = val.match(/PrivateKey\s*=\s*([A-Za-z0-9+/=]{40,48})/i);
      if (m && m[1]) {
        try {
          const derived = await api().PublicKeyFromPrivate(m[1].trim());
          if (derived) {
            pubEl.value = derived;
            pubEl.style.color = 'var(--accent2)';
          }
        } catch (e) {
          pubEl.value = '(özel anahtar geçersiz)';
          pubEl.style.color = 'var(--bad)';
        }
      }

      const msg = await api().ValidateConfig(val);
      validEl.className = 'validity ' + (msg ? 'bad' : 'ok');
      validEl.textContent = msg ? msg : 'Yapılandırma geçerli';
    };

    textEl.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(check, 250); });
    setTimeout(() => { nameEl.focus(); check(); }, 50);

    const ok = await modal({
      title: 'Yeni WireGuard tüneli oluştur',
      wide: true,
      body: wrap,
      actions: [
        { label: 'Vazgeç', value: false },
        { label: 'Tüneli Kaydet', cls: 'primary', value: true },
      ],
    });

    if (!ok) return;

    const name = nameEl.value.trim();
    if (!name) {
      toast('Lütfen bir tünel adı girin.', 'warn');
      return;
    }

    const confText = textEl.value;
    const created = await guard(() => api().CreateTunnel(name, confText), 'Tünel oluşturuldu: ' + name);
    if (created && onSuccess) {
      await onSuccess(created);
    }
  }

  function startPolling(cb, ms = 2000) {
    let busy = false;
    const tick = async () => {
      if (busy) return;
      busy = true;
      try { await cb(); } catch (e) { /* keep polling */ }
      busy = false;
    };
    tick();
    return setInterval(tick, ms);
  }

  window.PWG = { api, icons, esc, fmtBytes, fmtAgo, fmtUptime, toast, guard, modal, confirmBox, errText, serviceInfo, renderBanner, startPolling, copyToClipboard, openCreateTunnelModal };
})();
