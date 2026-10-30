(function () {
  const { api, icons, esc, fmtBytes, fmtAgo, fmtUptime, toast, guard, modal, confirmBox, serviceInfo, renderBanner, startPolling, copyToClipboard, openCreateTunnelModal } = window.PWG;
  const $ = (id) => document.getElementById(id);

  let state = null;
  let selected = null;           // profile name
  const busy = new Set();        // names with a connect/disconnect in progress
  let filter = '';

  $('logo').innerHTML = icons.shield;
  $('createTunnelBtn').innerHTML = icons.plus + 'Yeni Tünel';
  $('importBtn').innerHTML = icons.folder;
  $('exportAllBtn').innerHTML = icons.export;
  $('pasteBtn').innerHTML = icons.file;

  const tunnelNameOf = (t) => (t && (t.tunnel_name || t.tunnelName || ''));
  const tunnelOf = (name) => (state && state.tunnels ? state.tunnels.find((t) => tunnelNameOf(t).toLowerCase() === (name || '').toLowerCase()) : null);

  function chips(list, cls = '') {
    return list && list.length ? `<div class="chips">${list.map((x) => `<span class="chip mono ${cls}">${esc(x)}</span>`).join('')}</div>` : '<span class="chip">—</span>';
  }

  // "192.168.0.0/22 → 198.18.0.0/22"  ->  { real: '192.168.1.1', virt: '198.18.1.1' } (same host offset)
  function mapExample(m) {
    const parts = String(m).split('→').map((s) => s.trim());
    if (parts.length !== 2) return null;
    const toNum = (ip) => ip.split('/')[0].split('.').reduce((a, o) => a * 256 + Number(o), 0);
    const toIp = (n) => [24, 16, 8, 0].map((s) => Math.floor(n / 2 ** s) % 256).join('.');
    const bits = Number(parts[0].split('/')[1]);
    const host = Math.min(2 ** (32 - bits) - 2, 257); // a typical address like x.x.1.1 when the range is big enough
    return { real: toIp(toNum(parts[0]) + host), virt: toIp(toNum(parts[1]) + host) };
  }

  // ------------------------------------------------------------ list
  // The UI is polled every 2s. To avoid flicker, lost scroll position and replaying
  // animations, a part of the DOM is only rebuilt when the data it shows changed.
  let listSig = '';
  let detailSig = '';
  let lastDetailSelected = null;

  function renderList() {
    const q = filter.trim().toLowerCase();
    const items = state.profiles.filter((p) => !q || p.name.toLowerCase().includes(q) || (p.endpoint || '').toLowerCase().includes(q));
    $('count').textContent = state.profiles.length;

    const sig = JSON.stringify([
      state.profiles.length, q, selected,
      items.map((p) => [p.name, p.endpoint, p.error, p.external, !!tunnelOf(p.name), busy.has(p.name)]),
    ]);
    if (sig === listSig) return;
    listSig = sig;

    const list = $('list');
    const scroll = list.scrollTop;

    if (!state.profiles.length) {
      list.innerHTML = `<div class="empty"><b>Henüz profil yok</b>.conf, .ovpn veya .zip dosyalarını içe aktararak başlayın.</div>`;
      return;
    }
    if (!items.length) { list.innerHTML = `<div class="empty">Eşleşen profil yok.</div>`; return; }

    list.innerHTML = items.map((p) => {
      const on = !!tunnelOf(p.name);
      const b = busy.has(p.name);
      const protoTag = p.proto === 'openvpn' ? '<span class="tag ovpn">OVPN</span>' : '<span class="tag wg">WG</span>';
      return `<div class="item ${on ? 'on' : ''} ${p.name === selected ? 'sel' : ''}" data-name="${esc(p.name)}">
        <div class="meta">
          <div class="nm"><span class="nm-text" title="${esc(p.name)}">${esc(p.name)}</span> ${protoTag}${p.external ? ' <span class="tag" title="Kitaplık dışındaki dosya">dosya</span>' : ''}</div>
          <div class="ep ${p.error ? 'err' : ''}">${esc(p.error ? 'Geçersiz yapılandırma' : p.endpoint || '—')}</div>
        </div>
        <label class="switch ${b ? 'busy' : ''}" title="${on ? 'Bağlantıyı kes' : 'Bağlan'}">
          <input type="checkbox" data-sw="${esc(p.name)}" ${on ? 'checked' : ''} ${b || p.error ? 'disabled' : ''}>
          <span></span>
        </label>
      </div>`;
    }).join('');
    list.scrollTop = scroll;
  }

  // Live counters are updated in place (no rebuild), so they never flicker.
  function updateLive(t) {
    if (!t) return;
    const set = (id, v) => { const el = $(id); if (el && el.textContent !== v) el.textContent = v; };
    set('stUptime', fmtUptime(t.connected_at));
    set('stHs', fmtAgo(t.last_handshake));
    set('stRx', fmtBytes(t.rx_bytes));
    set('stTx', fmtBytes(t.tx_bytes));
  }

  // ------------------------------------------------------------ detail
  function renderDetail() {
    const body = $('detailBody');
    const p = state.profiles.find((x) => x.name === selected);
    const t = p ? tunnelOf(p.name) : null;

    const sig = JSON.stringify(p
      ? [selected, p.name, p.error, p.endpoint, p.addresses, p.dns, p.allowedIps, p.transport, p.path, p.external,
         !!t, t && t.addresses, t && t.dns_proxies, t && t.mappings, busy.has(p.name), state.service === 'running']
      : [null, state.tunnels.map((x) => [tunnelNameOf(x), x.addresses]), state.profiles.length]);

    if (sig === detailSig) { updateLive(t); return; }
    detailSig = sig;

    // entrance animation only when another profile was selected, not on every data change
    const fade = selected !== lastDetailSelected ? ' fade' : '';
    lastDetailSelected = selected;

    if (!p) {
      if (!state.tunnels.length) {
        body.innerHTML = `<div class="empty${fade}"><b>Bir profil seçin</b>Soldaki listeden bir profil seçin veya yeni yapılandırma içe aktarın.</div>`;
        return;
      }
      body.innerHTML = `<div class="card${fade}"><h3>Aktif tüneller</h3><div class="sum">${state.tunnels.map((x) =>
        `<div class="row"><span class="dot ok"></span><b>${esc(tunnelNameOf(x))}</b><span>${esc((x.addresses || []).join(', '))}</span></div>`).join('')}</div></div>`;
      return;
    }

    const b = busy.has(p.name);
    const sub = b ? (t ? 'Kapatılıyor…' : 'Bağlanıyor…')
      : t ? `Bağlı · ${(t.addresses || []).join(', ')}`
      : p.error ? 'Yapılandırma geçersiz' : 'Bağlı değil';

    body.innerHTML = `
      <div class="dhead${fade}">
        <div class="ico ${t ? 'on' : ''}">${icons.shield}</div>
        <div class="grow"><h1>${esc(p.name)}</h1><div class="st ${t && !b ? 'on' : ''}">${esc(sub)}</div></div>
        <div class="acts">
          <button class="btn" id="logBtn" title="Bağlantı günlüğünü görüntüle">${icons.file}Günlük</button>
          <button class="btn" id="editBtn" ${b ? 'disabled' : ''} title="${t ? 'Düzenlemek için önce bağlantıyı kesin' : 'Yapılandırmayı düzenle'}">${icons.edit}Düzenle</button>
          <button class="btn" id="exportBtn" ${b ? 'disabled' : ''} title="Profili dışa aktar">${icons.export}Dışa aktar</button>
          <button class="btn primary" id="toggleBtn" ${b || p.error || state.service !== 'running' ? 'disabled' : ''}>${icons.power}${t ? 'Bağlantıyı kes' : 'Bağlan'}</button>
          <button class="btn danger" id="deleteBtn" ${t || b ? 'disabled' : ''} title="${p.external ? 'Listeden kaldır (dosya silinmez)' : 'Profili sil'}">${icons.trash}</button>
        </div>
      </div>

      ${t ? `<div class="card${fade}"><h3>Canlı durum</h3><div class="stats cols3">
        <div class="stat"><div class="l">Süre</div><div class="v mono" id="stUptime"></div></div>
        <div class="stat"><div class="l">Son el sıkışma</div><div class="v" id="stHs"></div></div>
        <div class="stat"><div class="l">İndirilen</div><div class="v" id="stRx"></div></div>
        <div class="stat"><div class="l">Gönderilen</div><div class="v" id="stTx"></div></div>
      </div>
      ${t.mappings && t.mappings.length ? `<div class="note map">
        <b>Adres eşleme</b> ${chips(t.mappings, 'accent')}
        <span>Bu ağ başka bir tünelle aynı adresleri kullanıyor. Bu ağdaki bir bilgisayara <b>sanal adresle</b> ulaşın.
        ${t.mappings.map(mapExample).filter(Boolean).slice(0, 2).map((e) => `Örnek: <code>${esc(e.real)}</code> yerine <code>${esc(e.virt)}</code>.`).join(' ')}</span>
      </div>` : ''}
      ${t.dns_proxies && t.dns_proxies.length ? `<div class="note">
        <b>DNS vekili</b> ${chips(t.dns_proxies, 'accent')}
        <span>Bu DNS adresi başka bir tünelle çakışıyor veya eşlenen bir ağda; sorgular bu tünelin arayüzüne zorlanarak gönderiliyor.</span>
      </div>` : ''}
      </div>` : ''}

      <div class="card${fade}">
        <h3>Yapılandırma</h3>
        ${p.error ? `<span class="chip" style="color:var(--bad)">${esc(p.error)}</span>` : `
        <dl class="kv">
          <dt>Sunucu</dt><dd>${chips(p.endpoint ? [p.endpoint] : [], 'accent')}</dd>
          <dt>Adres</dt><dd>${chips(p.addresses)}</dd>
          <dt>DNS</dt><dd>${chips(p.dns)}</dd>
          <dt>Rotalar</dt><dd>${chips(p.allowedIps)}</dd>
          <dt>İletim</dt><dd>${chips([p.transport || 'udp'])}</dd>
          <dt>Kaynak</dt><dd><span class="chip mono" title="${esc(p.path)}">${p.external ? 'Dosya: ' + esc(p.path) : 'Profil kitaplığı'}</span></dd>
        </dl>`}
      </div>`;

    updateLive(t);
    const lb = $('logBtn'); if (lb) lb.onclick = () => showLog(p.name);
    const tb = $('toggleBtn'); if (tb) tb.onclick = () => toggle(p.name);
    const eb = $('editBtn'); if (eb) eb.onclick = () => editProfile(p.name);
    const xb = $('exportBtn'); if (xb) xb.onclick = () => exportProfile(p.name);
    const db = $('deleteBtn'); if (db) db.onclick = () => removeProfile(p.name);
  }

  function renderChrome() {
    const svc = serviceInfo(state.service);
    $('svcDot').className = 'dot ' + svc.cls;
    $('svcText').textContent = svc.text;
    const n = state.tunnels.length;
    $('activeDot').className = 'dot ' + (n ? 'ok' : '');
    $('activeText').textContent = n + ' aktif tünel';
    $('disconnectAll').classList.toggle('hidden', n === 0);
    $('ver').textContent = 'v' + state.version + ' · çoklu sürüm';
    renderBanner($('banner'), state, () => setTimeout(refresh, 2500));
  }

  function render() {
    if (!state) return;
    if (!selected || !state.profiles.some((p) => p.name === selected)) {
      const first = state.profiles.find((p) => tunnelOf(p.name)) || state.profiles[0];
      selected = first ? first.name : null;
    }
    renderChrome();
    renderList();
    renderDetail();
  }

  let initialInstallPromptShown = false;
  async function checkInitialInstall() {
    if (initialInstallPromptShown) return;
    if (state && state.service === 'notinstalled') {
      initialInstallPromptShown = true;
      const ok = await modal({
        title: '🛡️ PoseaVPN Servis Kurulumu',
        body: `
          <div style="line-height:1.6;font-size:13.5px;color:var(--text)">
            <p style="margin-bottom:12px">
              PoseaVPN'in güvenli VPN tünelleri açabilmesi, DNS yapılandırmasını yönetebilmesi ve
              sonrasında <b>yönetici şifresi sormadan</b> çalışabilmesi için sistem servisini kurması gerekir.
            </p>
            <div style="background:rgba(109,124,255,0.08);border:1px solid rgba(109,124,255,0.25);border-radius:8px;padding:10px 14px;color:var(--muted);font-size:12px;margin-bottom:6px">
              ℹ️ Kurulum için tek seferlik yönetici (Touch ID / parola) onayı istenecektir.
            </div>
          </div>
        `,
        actions: [
          { label: 'Şimdilik Geç', value: false },
          { label: 'Servisi Kur ve Başlat', cls: 'primary', value: true }
        ]
      });
      if (ok) {
        const res = await guard(() => api().ServiceInstall(), 'Yönetici onayı bekleniyor…');
        if (res !== undefined) {
          toast('PoseaVPN servisi başarıyla kuruldu ve başlatıldı!', 'ok');
          setTimeout(refresh, 1500);
        }
      }
    }
  }

  // ------------------------------------------------------------ actions
  async function refresh() {
    state = await api().GetState();
    checkInitialInstall();
    // do not rebuild the DOM while the user is typing in the search box
    render();
  }

  async function showLog(name) {
    let logText = 'Günlük yükleniyor…';
    try {
      logText = (await api().GetProfileLog(name)) || 'Kayıtlı günlük bulunmuyor.';
    } catch (e) {
      logText = 'Günlük alınamadı: ' + errText(e);
    }

    const wrap = document.createElement('div');
    wrap.innerHTML = `
      <div style="font-size:12.5px;color:var(--muted);margin-bottom:8px">
        <b>${esc(name)}</b> profilinin bağlantı ve motor (OpenVPN / WireGuard) günlüğü:
      </div>
      <pre class="log-view" id="logArea">${esc(logText)}</pre>
    `;

    setTimeout(() => {
      const area = wrap.querySelector('#logArea');
      if (area) area.scrollTop = area.scrollHeight;
    }, 100);

    const res = await modal({
      title: 'Bağlantı Günlüğü — ' + name,
      wide: true,
      body: wrap,
      primary: 'Kapat',
      cancel: 'Panoya Kopyala',
    });

    if (res === false) {
      try {
        await navigator.clipboard.writeText(logText);
        toast('Günlük panoya kopyalandı.', 'ok');
      } catch (_) {
        toast('Panoya kopyalanamadı.', 'warn');
      }
    }
  }

  async function promptAuthAndConnect(p) {
    let cred = { username: '', password: '' };
    try {
      cred = (await api().GetCredentials(p.name)) || cred;
    } catch (_) {}

    const wrap = document.createElement('div');
    wrap.innerHTML = `
      <div style="font-size:13px;color:var(--muted);margin-bottom:14px">
        <b>${esc(p.name)}</b> OpenVPN sunucusu kullanıcı adı ve şifre gerektiriyor.
      </div>
      <div class="field"><label>Kullanıcı Adı</label><input class="input" id="authU" value="${esc(cred.username || '')}" spellcheck="false" autocomplete="off" /></div>
      <div class="field" style="margin-top:12px"><label>Parola</label><input class="input" type="password" id="authP" value="${esc(cred.password || '')}" autocomplete="off" /></div>
      <label style="display:flex;align-items:center;gap:8px;margin-top:14px;font-size:12px;cursor:pointer">
        <input type="checkbox" id="authSave" ${cred.username ? 'checked' : ''} />
        <span>Giriş bilgilerini bu profil için hatırla</span>
      </label>
    `;
    const res = await modal({
      title: 'OpenVPN Giriş Bilgileri',
      body: wrap,
      primary: 'Bağlan',
      cancel: 'İptal',
      focus: cred.username ? wrap.querySelector('#authP') : wrap.querySelector('#authU'),
    });
    if (!res) return;
    const u = wrap.querySelector('#authU').value.trim();
    const pw = wrap.querySelector('#authP').value;
    const save = wrap.querySelector('#authSave').checked;
    if (!u) {
      toast('Kullanıcı adı boş bırakılamaz.', 'bad');
      return;
    }

    busy.add(p.name);
    render();
    try {
      const msg = await api().ConnectProfileWithAuth(p.name, u, pw, save);
      if (msg) toast(msg, 'ok');
    } catch (err) {
      const errStr = String(err || '').replace('service error: ', '');
      toast(errStr, 'bad');
      // If failed, auto-offer log view
      if (errStr.includes('AUTH_FAILED') || errStr.includes('TLS')) {
        setTimeout(() => showLog(p.name), 800);
      }
    } finally {
      busy.delete(p.name);
      await refresh();
    }
  }

  async function toggle(name) {
    if (busy.has(name)) return;
    if (state.service !== 'running') { toast('Önce arka plan servisini başlatın.', 'warn'); await refresh(); return; }
    const on = !!tunnelOf(name);
    if (on) {
      busy.add(name); render();
      try {
        await guard(() => api().Disconnect(name), `'${name}' kapatıldı`);
      } finally {
        busy.delete(name);
        await refresh();
      }
      return;
    }

    const p = state.profiles.find((x) => x.name === name);
    // If OpenVPN requires auth and has no saved creds, prompt directly
    if (p && p.proto === 'openvpn' && p.needsAuth) {
      let cred = null;
      try { cred = await api().GetCredentials(name); } catch (_) {}
      if (!cred || !cred.username || !cred.password) {
        await promptAuthAndConnect(p);
        return;
      }
    }

    busy.add(name); render();
    try {
      const msg = await api().ConnectProfile(name);
      if (msg) toast(msg, msg.includes('warning') ? 'warn' : 'ok');
    } catch (err) {
      const errStr = String(err || '');
      if (errStr.includes('AUTH_REQUIRED')) {
        await promptAuthAndConnect(p || { name });
      } else {
        const cleanErr = errStr.replace('service error: ', '');
        toast(cleanErr, 'bad');
        if (p && (cleanErr.includes('AUTH_FAILED') || cleanErr.includes('TLS'))) {
          setTimeout(() => showLog(name), 800);
        }
      }
    } finally {
      busy.delete(name);
      await refresh();
    }
  }

  async function removeProfile(name) {
    const p = state.profiles.find((x) => x.name === name);
    const ext = p && p.external;
    const ok = await confirmBox(
      ext ? 'Listeden kaldır' : 'Profili sil',
      ext ? `'${name}' listeden kaldırılacak. Dosyanın kendisi silinmez.` : `'${name}' profili silinecek. Bu işlem geri alınamaz.`,
      ext ? 'Kaldır' : 'Sil');
    if (!ok) return;
    const r = await guard(() => api().DeleteProfile(name).then(() => true), ext ? `'${name}' listeden kaldırıldı` : `'${name}' silindi`);
    if (r) { selected = null; await refresh(); }
  }

  async function importFiles() {
    const res = await guard(() => api().ImportConfigs());
    if (!res) return;
    if (res.imported.length) {
      toast(`${res.imported.length} profil içe aktarıldı` + (res.imported.length > 1 ? ': ' + res.imported.slice(0, 4).join(', ') + (res.imported.length > 4 ? '…' : '') : ''), 'ok');
      selected = res.imported[0];
    }
    res.errors.slice(0, 5).forEach((e) => toast(e, 'bad'));
    if (res.errors.length > 5) toast(`… ve ${res.errors.length - 5} hata daha`, 'bad');
    await refresh();
  }

  // Editor: edits the raw .conf text and the profile name, with live validation.
  async function editProfile(name) {
    let text = await guard(() => api().GetProfileText(name));
    if (text === undefined) return;
    let newName = name;

    for (;;) {
      const wrap = document.createElement('div');
      wrap.innerHTML = `
        <div class="field"><label>Profil adı</label><input class="input" id="eName" spellcheck="false" /></div>
        <div class="field" id="ePubField" style="margin-top:12px;display:none">
          <label style="display:flex;justify-content:space-between;align-items:center">
            <span>Açık anahtar (Public Key) — <span style="color:var(--accent2);font-weight:500">Sunucu yöneticisine verilecek</span></span>
          </label>
          <div style="display:flex;gap:8px;align-items:center;margin-top:4px">
            <input class="input mono" id="ePubKey" readonly style="flex:1;background:rgba(0,0,0,.35);cursor:text;color:var(--accent2);font-weight:600" />
            <button class="btn sm" id="eCopyPub" type="button">${icons.copy} Kopyala</button>
            <button class="btn sm" id="eRegenPub" type="button" title="Yeni bir anahtar çifti üret">${icons.refresh} Yeni Anahtar</button>
          </div>
        </div>
        <div class="field" style="margin-top:12px"><label>Yapılandırma (.conf / .ovpn)</label>
          <textarea class="textarea tall" id="eText" spellcheck="false"></textarea>
          <div class="validity" id="eValid"></div></div>`;
      const nameEl = wrap.querySelector('#eName');
      const textEl = wrap.querySelector('#eText');
      const validEl = wrap.querySelector('#eValid');
      const pubField = wrap.querySelector('#ePubField');
      const pubEl = wrap.querySelector('#ePubKey');
      const copyPubBtn = wrap.querySelector('#eCopyPub');
      const regenPubBtn = wrap.querySelector('#eRegenPub');
      nameEl.value = newName;
      textEl.value = text;

      copyPubBtn.onclick = () => copyToClipboard(pubEl.value);
      regenPubBtn.onclick = async () => {
        try {
          const newKp = await api().GenerateKeyPair();
          pubEl.value = newKp.publicKey;
          pubEl.style.color = 'var(--accent2)';
          const cur = textEl.value;
          const re = /(PrivateKey\s*=\s*)[^\r\n]+/i;
          if (re.test(cur)) {
            textEl.value = cur.replace(re, `$1${newKp.privateKey}`);
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
          pubField.style.display = 'block';
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
        } else {
          pubField.style.display = 'none';
        }
        const msg = await api().ValidateConfig(val);
        validEl.className = 'validity ' + (msg ? 'bad' : 'ok');
        validEl.textContent = msg ? msg : 'Yapılandırma geçerli';
      };
      textEl.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(check, 250); });
      setTimeout(check, 0);

      const ok = await modal({
        title: 'Yapılandırmayı düzenle',
        wide: true,
        sticky: true,
        body: wrap,
        actions: [{ label: 'Vazgeç', value: false }, { label: 'Kaydet', cls: 'primary', value: true }],
      });
      newName = nameEl.value;
      text = textEl.value;
      if (!ok) return;

      const final = await guard(() => api().SaveProfileText(name, newName, text), 'Kaydedildi');
      if (final !== undefined) { selected = final; await refresh(); return; }
      // save failed (toast already shown): reopen with the edited text kept
    }
  }

  async function pasteConfig() {
    const wrap = document.createElement('div');
    wrap.innerHTML = `
      <div class="field"><label>Profil adı</label><input class="input" id="pName" placeholder="ör. sirket-vpn" /></div>
      <div class="field" style="margin-top:12px"><label>Yapılandırma (.conf içeriği)</label>
        <textarea class="textarea" id="pText" spellcheck="false" placeholder="[Interface]&#10;PrivateKey = …&#10;Address = 10.0.0.2/24&#10;&#10;[Peer]&#10;PublicKey = …&#10;Endpoint = host:51820&#10;AllowedIPs = 10.0.0.0/24"></textarea></div>`;
    const ok = await modal({ title: 'Yapılandırma yapıştır', body: wrap, actions: [{ label: 'Vazgeç', value: false }, { label: 'Kaydet', cls: 'primary', value: true }] });
    if (!ok) return;
    const name = wrap.querySelector('#pName').value.trim();
    const text = wrap.querySelector('#pText').value;
    if (!name) { toast('Profil adı gerekli.', 'warn'); return; }
    const saved = await guard(() => api().ImportText(name, text), 'Profil kaydedildi');
    if (saved) { selected = saved; await refresh(); }
  }

  // ------------------------------------------------------------ events
  $('list').addEventListener('click', (e) => {
    const swLabel = e.target.closest('.switch');
    if (swLabel) {
      // handle the click ourselves; the checkbox must not flip before the backend answers
      e.preventDefault();
      e.stopPropagation();
      const input = swLabel.querySelector('[data-sw]');
      if (input && !input.disabled) toggle(input.dataset.sw);
      return;
    }
    const it = e.target.closest('.item');
    if (it) { selected = it.dataset.name; render(); }
  });

  async function exportProfile(name) {
    const res = await guard(() => api().ExportProfile(name));
    if (res) toast(`'${res}' olarak dışa aktarıldı`, 'ok');
  }

  async function exportAll() {
    const res = await guard(() => api().ExportAllZip());
    if (res) toast(res, 'ok');
  }

  $('search').addEventListener('input', (e) => { filter = e.target.value; if (state) renderList(); });
  $('createTunnelBtn').onclick = () => openCreateTunnelModal(async (created) => { if (created && created.name) selected = created.name; await refresh(); });
  $('importBtn').onclick = importFiles;
  $('exportAllBtn').onclick = exportAll;
  $('pasteBtn').onclick = pasteConfig;
  $('disconnectAll').onclick = async () => {
    if (!(await confirmBox('Tümünü kes', 'Tüm aktif tüneller kapatılacak.', 'Tümünü kes'))) return;
    await guard(() => api().Disconnect(''), 'Tüm tüneller kapatıldı');
    await refresh();
  };
  const rb = $('restartBtn');
  if (rb) {
    rb.onclick = async () => {
      const ok = await confirmBox('Servisi Yeniden Başlat', 'PoseaVPN arka plan servisi güncellenecek ve yeniden başlatılacak. Devam edilsin mi?', 'Yeniden Başlat');
      if (!ok) return;
      toast('Yönetici onayı (UAC) bekleniyor… Lütfen ekrandaki Evet butonuna basın.', 'warn');
      const res = await guard(() => api().ServiceRestart(), 'Yönetici onayı bekleniyor…');
      if (res !== undefined) setTimeout(refresh, 2500);
    };
  }
  $('uninstallBtn').onclick = async () => {
    if (!(await confirmBox('Servisi kaldır', 'Arka plan servisi durdurulup sistemden kaldırılacak. Devam edilsin mi?', 'Kaldır'))) return;
    const ok = await guard(() => api().ServiceUninstall(), 'Yönetici onayı bekleniyor…');
    if (ok !== undefined) setTimeout(refresh, 2500);
  };

  setInterval(() => {
    if (!state || !selected) return;
    updateLive(tunnelOf(selected));
  }, 1000);

  startPolling(refresh, 2000);
})();
