(function () {
  const { api, icons, esc, fmtBytes, fmtAgo, fmtUptime, toast, guard, modal, confirmBox, serviceInfo, renderBanner, startPolling, openCreateTunnelModal } = window.PWG;
  const $ = (id) => document.getElementById(id);

  let state = null;       // last GetState()
  let pending = false;    // connect/disconnect in progress

  $('logo').innerHTML = icons.shield;
  $('powerIcon').innerHTML = icons.power;
  $('cfgIco').innerHTML = icons.file;
  $('createTunnelBtn').innerHTML = icons.plus + 'Yeni Tünel';
  $('chooseBtn').innerHTML = icons.folder + 'Seç';

  const tunnelNameOf = (t) => (t && (t.tunnel_name || t.tunnelName || ''));
  const activeTunnel = () => {
    if (!state || !state.tunnels.length) return null;
    const last = state.lastConfig && state.lastConfig.name.toLowerCase();
    return state.tunnels.find((t) => tunnelNameOf(t).toLowerCase() === last) || state.tunnels[0];
  };

  function chips(list, cls = '') {
    return list && list.length ? `<div class="chips">${list.map((x) => `<span class="chip mono ${cls}">${esc(x)}</span>`).join('')}</div>` : '<span class="chip">—</span>';
  }

  function render() {
    if (!state) return;
    const svc = serviceInfo(state.service);
    $('svcDot').className = 'dot ' + svc.cls;
    $('svcText').textContent = svc.text;
    $('ver').textContent = 'v' + state.version + ' · tekli sürüm';
    renderBanner($('banner'), state, () => setTimeout(refresh, 2500));

    const t = activeTunnel();
    const cfg = state.lastConfig;
    const power = $('power');
    const ready = state.service === 'running';

    power.className = 'power' + (pending ? ' connecting' : t ? ' on' : '') + (ready ? '' : ' disabled');

    const st = $('statusText');
    st.className = 'status' + (t && !pending ? ' on' : '');
    if (pending) {
      st.textContent = t ? 'Kapatılıyor…' : 'Bağlanıyor…';
      $('statusSub').textContent = cfg ? cfg.name : '';
    } else if (t) {
      st.textContent = 'Bağlı';
      $('statusSub').textContent = (t.addresses || []).join(', ') + (t.endpoints && t.endpoints[0] ? '  →  ' + t.endpoints[0] : '');
    } else {
      st.textContent = 'Bağlı değil';
      $('statusSub').textContent = !ready ? 'Önce arka plan servisini başlatın' : cfg ? 'Bağlanmak için düğmeye dokunun' : 'Önce bir yapılandırma seçin';
    }

    // config card
    if (cfg) {
      const protoTag = cfg.proto === 'openvpn' ? ' <span class="tag ovpn">OVPN</span>' : ' <span class="tag wg">WG</span>';
      $('cfgName').innerHTML = esc(cfg.name) + protoTag;
      $('cfgPath').textContent = cfg.path;
      $('cfgPath').title = cfg.path;
      const html = cfg.error
        ? `<dt>Hata</dt><dd><span class="chip" style="color:var(--bad)">${esc(cfg.error)}</span></dd>`
        : `<dt>Sunucu</dt><dd>${chips(cfg.endpoint ? [cfg.endpoint] : [], 'accent')}</dd>
           <dt>Adres</dt><dd>${chips(cfg.addresses)}</dd>
           <dt>DNS</dt><dd>${chips(cfg.dns)}</dd>
           <dt>Rotalar</dt><dd>${chips(cfg.allowedIps)}</dd>
           <dt>İletim</dt><dd>${chips([cfg.transport || 'udp'])}</dd>`;
      // only touch the DOM when the content changed (no flicker, text selection survives)
      if ($('cfgDetails').dataset.html !== html) {
        $('cfgDetails').dataset.html = html;
        $('cfgDetails').innerHTML = html;
      }
    } else {
      $('cfgName').textContent = 'Yapılandırma seçilmedi';
      $('cfgPath').textContent = '.conf veya .ovpn dosyanızı seçin';
      $('cfgDetails').dataset.html = '';
      $('cfgDetails').innerHTML = '';
    }
    $('createTunnelBtn').disabled = !!t || pending;
    $('chooseBtn').disabled = !!t || pending;
    const xb = $('exportBtn');
    if (xb) {
      xb.style.display = cfg ? '' : 'none';
      xb.innerHTML = icons.export + 'Dışa aktar';
      xb.disabled = pending;
      xb.onclick = async () => {
        if (!state.lastConfig) return;
        const res = await guard(() => api().ExportProfile(state.lastConfig.name));
        if (res) toast(`'${res}' olarak dışa aktarıldı`, 'ok');
      };
    }
    const lb = $('logBtn');
    if (lb) {
      lb.style.display = cfg ? '' : 'none';
      lb.innerHTML = icons.file + 'Günlük';
      lb.disabled = pending;
      lb.onclick = () => {
        if (state.lastConfig) showLog(state.lastConfig.name);
      };
    }

    // live stats
    $('statsCard').classList.toggle('hidden', !t);
    if (t) {
      $('stHs').textContent = fmtAgo(t.last_handshake);
      $('stRx').textContent = fmtBytes(t.rx_bytes);
      $('stTx').textContent = fmtBytes(t.tx_bytes);
      $('stUptime').textContent = fmtUptime(t.connected_at);
    }
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

  async function refresh() {
    state = await api().GetState();
    checkInitialInstall();
    render();
  }

  $('power').onclick = async () => {
    if (pending || !state) return;
    if (state.service !== 'running') { toast('Önce arka plan servisini başlatın.', 'warn'); return; }
    const t = activeTunnel();
    if (!t && !state.lastConfig) { $('chooseBtn').click(); return; }

    pending = true; render();
    try {
      if (t) {
        await guard(() => api().Disconnect(tunnelNameOf(t)), 'Bağlantı kesildi');
      } else {
        try {
          const msg = await api().ConnectLast();
          if (msg) toast(msg, msg.includes('warning') ? 'warn' : 'ok');
        } catch (err) {
          const errStr = String(err || '');
          if (errStr.includes('AUTH_REQUIRED') && state.lastConfig) {
            await promptAuthAndConnect(state.lastConfig);
            return;
          } else {
            toast(errStr.replace('service error: ', ''), 'bad');
          }
        }
      }
    } finally {
      pending = false;
      await refresh();
    }
  };

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

  async function promptAuthAndConnect(cfg) {
    let cred = { username: '', password: '' };
    try {
      cred = (await api().GetCredentials(cfg.name)) || cred;
    } catch (_) {}

    const wrap = document.createElement('div');
    wrap.innerHTML = `
      <div style="font-size:13px;color:var(--muted);margin-bottom:14px">
        <b>${esc(cfg.name)}</b> OpenVPN sunucusu kullanıcı adı ve şifre gerektiriyor.
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

    pending = true; render();
    try {
      const msg = await api().ConnectProfileWithAuth(cfg.name, u, pw, save);
      if (msg) toast(msg, 'ok');
    } catch (err) {
      const errStr = String(err || '').replace('service error: ', '');
      toast(errStr, 'bad');
      if (errStr.includes('AUTH_FAILED') || errStr.includes('TLS')) {
        setTimeout(() => showLog(cfg.name), 800);
      }
    } finally {
      pending = false;
      await refresh();
    }
  }

  $('createTunnelBtn').onclick = () => openCreateTunnelModal(async (created) => { toast('Tünel hazır: ' + created.name, 'ok'); await refresh(); });
  $('chooseBtn').onclick = async () => {
    const p = await guard(() => api().ChooseConfig());
    if (p) { toast('Yapılandırma hatırlandı: ' + p.name, 'ok'); await refresh(); }
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

  // uptime ticks every second without hitting the backend
  setInterval(() => {
    const t = activeTunnel();
    if (t && !pending) {
      $('stUptime').textContent = fmtUptime(t.connected_at);
      $('stHs').textContent = fmtAgo(t.last_handshake);
    }
  }, 1000);

  startPolling(refresh, 2000);
})();
