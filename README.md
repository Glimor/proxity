# Proxity

Uygulama bazında proxy yönlendirmesi yapan bir Windows trafik motoru —
hangi programın hangi SOCKS5 proxy üzerinden çıkacağını kural bazında
belirlersiniz; kalan trafik normal yoluna devam eder.

**Platform:** Windows 10/11 x64 · **Lisans:** GPL-3.0 · **Dil:** saf Go
(`CGO_ENABLED=0`, WinDivert dahil sürücü dışında yerel bağımlılık yok)

> Ekran görüntüleri yakında eklenecek (`docs/screenshots/`).

## Özellikler

- **Uygulama bazlı yönlendirme** — exe veya klasör kuralı: proxy, doğrudan
  veya engelle. Sistem genelinde proxy ayarına dokunmaz.
- **Kill switch (fail-closed)** — WFP üzerinde kalıcı BLOCK filtreleri;
  motor durursa korumalı uygulamaların trafiği anında kesilir. Küresel kill
  switch tüm makinenin internetini kesebilir (LAN izinli).
- **UDP sızıntı koruması** — UDP proxylenemediği için, kill switch'siz
  proxy kurallarında uzak UDP otomatik engellenir; TCP etkilenmez.
- **Güvenli DNS** — yerel 127.0.0.1 yönlendirici; sorgular proxy üzerinden
  gider, düz metin DNS makineden çıkmaz.
- **Kural dışı sızıntı yok** — yansıtılmış SYN'ler için servisin kendisine
  ait kalıcı RECV_ACCEPT izni; servis çökerse dinamik izinler düşer,
  bloklar kalır.
- **İstatistikler** — canlı grafik, aktif bağlantılar, engellenenler günlüğü,
  günlük exe/proxy toplamları, CSV dışa aktarma, 400 günlük tutma.
- **Tepsi uygulaması + sihirbaz** — ilk kurulum 3 adımda; Türkçe/İngilizce
  arayüz, koyu/açık tema.

## Kurulum

`Proxity-amd64-installer.exe` dosyasını indirip çalıştırın. Kurulum
**yönetici izni** ister çünkü arka planda `ProxityEngine` Windows servisini
kurar ve WinDivert çekirdek sürücüsünü yükler — bunlar normal kullanıcı
haklarıyla yapılamaz.

İmzasız derleme olduğu için SmartScreen uyarısı çıkabilir; "Ek bilgi →
Yine de çalıştır" ile devam edin.

Kaldırma: Ayarlar → Uygulamalar'dan veya kurulum klasöründeki
`uninstall.exe` ile. Kaldırıcı servisi durdurup WFP filtrelerini ve DNS
atamasını geri alır; **`%ProgramData%\Proxity` (kurallar, proxy'ler,
istatistikler) de silinir** — saklamak istiyorsanız önce Ayarlar → Proxy'leri
dışa aktar.

## Hızlı başlangıç (3 adım)

1. **Proxy ekleyin** — Proxy'ler → Yeni: SOCKS5 adresi, port (varsa
   kullanıcı adı/parola; parola DPAPI ile şifrelenir).
2. **Kural ekleyin** — Kurallar → Yeni: exe veya klasör seçin, eylemi
   "Proxy" yapın, profili seçin. İsteğe bağlı kill switch açın.
3. **Motoru başlatın** — Pano'daki anahtarla. Artık o uygulama proxy'den
   çıkar; diğerleri normal bağlanır.

## Mimari (kuşbakışı)

```
┌──────────────┐   named pipe (JSON-RPC)   ┌────────────────────────┐
│ proxity.exe  │ ════════════════════════► │ proxity-service.exe    │
│ Wails GUI    │                           │ (SYSTEM, SCM servisi)  │
│ Vue3 + tray  │                           │                        │
└──────────────┘                           │  appsvc ── sqlite DB   │
                                           │    │                   │
                                           │    ├─► killswitch ─►   │
                                           │    │     WFP (fwpuclnt)│
                                           │    │   kalıcı BLOCK +  │
                                           │    │   dinamik PERMIT  │
                                           │    │                   │
                                           │    └─► engine ──►      │
                                           │      WinDivert capture │
                                           │      SYN yansıtma →    │
                                           │      127.0.0.1:37890   │
                                           │      SOCKS5 dial →     │
                                           │      upstream relay    │
                                           └────────────────────────┘
```

Ayrıntılar: [docs/architecture.md](docs/architecture.md),
[docs/user-guide-tr.md](docs/user-guide-tr.md),
[docs/dev.md](docs/dev.md).

## Kaynaktan derleme

Gereksinimler: Go (go.mod'daki sürüm), Node.js 20+, Wails CLI
(`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`) ve
kurulum paketi için NSIS (`winget install NSIS.NSIS`).

```powershell
# Tek komut — servis + GUI + NSIS kurulum paketi, hepsi build\bin'de:
powershell -ExecutionPolicy Bypass -File build.ps1 -Version 0.1.0
```

Geliştirme için ayrı ayrı:

```powershell
# frontend
cd frontend; npm ci; npm run build; cd ..

# servis (konsol modunda çalıştırma için yönetici konsolu)
go build -o build\bin\proxity-service.exe .\cmd\proxity-service
proxity-service.exe -console      # veya -engine ile sadece motor

# GUI
wails build            # build\bin\proxity.exe
wails dev              # sıcak yeniden yükleme
```

Testler: `go test ./...` — yönetici/entegrasyon testleri `PROXITY_INTEG=1`
ortam değişkeniyle açılır (bkz. `tests/`, `internal/engine/load_test.go`).

## Lisans ve atıflar

- **Proxity**: [GPL-3.0](LICENSE)
- **WinDivert** (sürücü, `third_party/windivert/`): LGPL —
  [basil00/WinDivert](https://github.com/basil00/WinDivert),
  lisans metni `third_party/windivert/LICENSE`
- `internal/wfp` ve `internal/windivert` bağlama kodu, MIT lisanslı
  WireGuard for Windows projesinden uyarlanmıştır (dosya başlıklarındaki
  NOTICE'a bakınız).

---

## English (short)

**Proxity** routes TCP traffic of selected applications through SOCKS5
proxies on Windows 10/11 — per-exe/folder rules (proxy, direct, block), a
fail-closed WFP kill switch (per-app and global), UDP-leak blocking for
non-kill-switch proxy rules, optional proxied DNS, and a Wails GUI
(tray, TR/EN, stats + CSV export). A SYSTEM service
(`proxity-service.exe`) owns the WinDivert capture engine and persistent
WFP filters; the GUI talks to it over a named pipe.

Install: run the NSIS installer **as administrator** — it registers the
service and deploys the WinDivert driver. Uninstall removes the service,
WFP filters and `%ProgramData%\Proxity`.

Build: `powershell -ExecutionPolicy Bypass -File build.ps1 -Version 0.1.0`
(needs Go, Node, Wails CLI, NSIS). License: GPL-3.0; WinDivert is LGPL.
