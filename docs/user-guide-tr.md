# Proxity Kullanım Kılavuzu

## Kurulum

1. `Proxity-amd64-installer.exe`'i çalıştırın → UAC iznini onaylayın.
   Kurulum `proxity.exe` (arayüz), `proxity-service.exe` (arka plan
   servisi) ve WinDivert sürücü dosyalarını `%ProgramFiles%\Proxity\Proxity`
   altına kopyalar ve `ProxityEngine` servisini kurup başlatır.
2. İlk açılışta karşılama sihirbazı dil ve temel ayarları yapar.
3. Servis durumunu Pano ekranından izleyin; servis ulaşılamıyorsa
   "Servisi kur" düğmesi UAC ile `-install` çalıştırır.

> SmartScreen uyarısı normaldir — derlemeler imzasızdır.

## Temel kavramlar

| Kavram | Anlamı |
|--------|--------|
| **Profil** | Bir SOCKS5 proxy: host, port, isteğe bağlı kullanıcı adı/parola. Parola DPAPI ile makineye kilitli şifrelenir. |
| **Kural** | Bir exe veya klasör + eylem (Proxy / Doğrudan / Engelle) + isteğe bağlı kill switch. Klasör kuralları isteğe bağlı alt klasörleri de kapsar. |
| **Motor** | WinDivert ile giden TCP SYN'lerini yakalar, kurala göre yansıtıp proxy'ler. |
| **Kill switch** | Motor ölürse trafiği kesen WFP blokları. Kural bazlı veya küresel. |

## Kurallar

- **Proxy**: bağlantılar seçilen profil üzerinden çıkar. Kill switch
  açıksa motor/proxy öldüğünde o uygulama internete çıkamaz; kapalıysa
  TCP doğrudan devam eder ama **uzak UDP yine de engellenir** (UDP
  proxylenemez; sızdırmaması için).
- **Doğrudan**: kuraldışı trafik gibi davranır; açıkça direct istediğiniz
  uygulamalar için.
- **Engelle**: o uygulama hiç çıkmasın (LAN/loopback yine serbest).

Aynı yola birden fazla kural yazılırsa çakışma uyarısı görürsünüz;
eşleşmede en spesifik/en yüksek öncelikli kural kazanır.

## Ayarlar

- **Servisi Windows ile başlat**: servis kurulumunu/kaldırma + başlangıç
  türünü (Otomatik/Elle) yönetir.
- **Küresel kill switch**: motor durursa TÜM makine internete kapalıdır;
  LAN erişimi ayrı bir anahtarla açık tutulabilir.
- **Güvenli DNS**: adaptör DNS'i 127.0.0.1'e çevrilir; sorgular proxy
  üzerinden çözülür, düz metin DNS çıkmaz. Upstream ve DNS için kullanılacak
  profil seçilebilir.
- **Bildirimler**: engellenen bağlantı ve proxy kopma/yeniden bağlanma
  uyarıları.
- **Dışa/içe aktarma**: profiller JSON olarak taşınabilir; parola dahil
  etmek isteğe bağlıdır (dosyayı güvenli saklayın).

## Kaldırma

`uninstall.exe` veya Windows Ayarlar → Uygulamalar. Kaldırıcı:

- `proxity-service.exe -uninstall` çalıştırır → servis durdurulur ve
  silinir, tüm WFP filtreleri/sağlayıcısı kaldırılır, adaptör DNS'i geri
  alınır;
- `%ProgramData%\Proxity` **silinir** (veritabanı, kurallar, profiller,
  istatistikler, günlükler). Saklamak için önce Ayarlar → Proxy'leri dışa
  aktarın.

## Sorun giderme

- **Servis ulaşılamıyor**: Pano'daki uyarıdan yeniden kurulum deneyin;
  `services.msc` → ProxityEngine durumuna bakın.
- **Bir uygulama proxy'den çıkmıyor**: kuralın yolu exe'nin tam yoluyla
  eşleşiyor mu (AppData altında çalışan güncelleyici değil gerçek exe?);
  bağlantılar yeni açılanlarda geçerlidir — uygulamayı yeniden başlatın.
- **Günlükler**: `%ProgramData%\Proxity\logs` — Ayarlar → Günlük düzeyi
  `debug` yapın.
- **Engellenenler ekranı**: kill switch bloklarının canlı listesi.
