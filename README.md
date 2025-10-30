# Opentify

[![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)](https://go.dev)
[![GUI-Fyne](https://img.shields.io/badge/GUI-Fyne%20v2-6e53a3)](https://fyne.io)
[![Audio-beep](https://img.shields.io/badge/Audio-faiface%2Fbeep-4c9a2a)](https://github.com/faiface/beep)
[![Platforms](https://img.shields.io/badge/Platforms-Linux%20%7C%20macOS%20%7C%20Windows-informational)](#platform-durumu)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)

Basit ve hızlı bir masaüstü müzik oynatıcı. Go ile yazıldı; arayüz için Fyne v2, ses çalma için faiface/beep kullanır. Uygulama `musicdb/` klasörünü (özyineli) tarar, desteklenen dosyaları listeler ve çalar.

---

## İçindekiler
- [Özellikler](#özellikler)
- [Platform durumu](#platform-durumu)
- [Ekran görüntüleri](#ekran-görüntüleri)
- [Kurulum](#kurulum)
- [Hızlı başlangıç](#hızlı-başlangıç)
- [Kullanım](#kullanım)
- [Mimarî ve Yapı](#mimarî-ve-yapı)
- [Geliştirme](#geliştirme)
- [Test](#test)
- [Sorun giderme](#sorun-giderme)
- [SSS](#sss)
- [Yol haritası](#yol-haritası)
- [Katkı](#katkı)
- [Teşekkür](#teşekkür)
- [Lisans](#lisans)

## Özellikler
- Masaüstünde yerel pencerede dosya listesi ve kontroller (Oynat/Duraklat/Durdur/Yenile)
- Otomatik medya tarama: `musicdb/` altında özyineli tarama ve sıralı liste
- Format desteği: MP3, WAV, FLAC, OGG (beep decoder’ları ile)
- Örnekleme oranına uygun hoparlör başlatma; dosyaya göre yeniden yapılandırma

## Platform durumu
- Masaüstü (Linux/macOS/Windows): ÇALMA desteklenir.
- Mobil (Android/iOS): Aynı API mevcut fakat şimdilik no‑op (oynatma yok). Gelecekte eklenecek.

## Ekran görüntüleri
- `docs/screenshots/` altına ekran görüntüleri ekleyebilirsiniz.
- Örnek: `docs/screenshots/main.png`

## Kurulum

### Gereksinimler
- Go 1.21+
- Derleyici ve grafik/ses bağımlılıkları (Fyne):
  - Linux: `gcc` veya `clang`, OpenGL sürücüleri
  - macOS: Xcode Command Line Tools
  - Windows: Mingw-w64 (veya MSVC) + uygun OpenGL sürücüleri

### Bağımlılıkları çekin
```bash path=null start=null
go mod tidy
```

## Hızlı başlangıç
```bash path=null start=null
# Geliştirme modunda çalıştır
go run .

# Veya ikili oluşturup çalıştırın
go build -o opentify ./
./opentify
```

## Kullanım
1. Müzik dosyalarınızı `musicdb/` klasörü altına yerleştirin (alt klasörler desteklenir).
2. Uygulamayı açın; dosyalar otomatik listelenir.
3. Bir dosya seçin ve oynatma kontrollerini kullanın.

Desteklenen uzantılar: `mp3`, `wav`, `flac`, `ogg`.

## Mimarî ve Yapı
- `main.go`: Fyne penceresi, liste ve oynatma kontrolleri; `musicdb/` taraması.
- `internal/player/`:
  - `player_desktop.go` (build tag: `!android && !ios`): faiface/beep + speaker ile gerçek oynatıcı (thread‑safe API: `Load`, `Play`, `Pause`, `Stop`, `CurrentFile`).
  - `player_mobile.go` (build tag: `android || ios`): Aynı API, şimdilik no‑op.

Build tag’ler platform davranışını belirler; masaüstü derlemelerinde gerçek oynatıcı kullanılır.

## Geliştirme
- Biçimlendirme ve temel statik analiz:
```bash path=null start=null
go fmt ./...
go vet ./...
```
- Modül bakımı:
```bash path=null start=null
go mod tidy
```

## Test
Şu an test bulunmuyor; eklendikçe aşağıdaki komutlar geçerlidir:
```bash path=null start=null
go test ./...
# Paket bazında
go test ./internal/player
# Test adıyla
go test -run '^TestName$' ./internal/player
```

## Sorun giderme
- Ses yok/bozuk: Sistem ses aygıtını/sürücüleri kontrol edin; farklı bir dosya deneyin.
- Takılma/bozulma: Farklı örnekleme oranlarına geçişte hoparlör yeniden kuruluyor; tekrar oynatmayı deneyin.
- Derleme hatası (Fyne/GL): Derleyici ve GL sürücülerinin kurulu olduğundan emin olun.

## SSS
- Mobilde neden çalmıyor? Şu an mobil oynatıcı iskelet hâlinde; yalnızca masaüstü desteklidir.
- `musicdb/` nereye konmalı? Proje kökünde bir klasördür; uygulama bu klasörü tarar.

## Yol haritası
- [ ] Mobilde gerçek oynatma
- [ ] Çalma listeleri ve sıralama
- [ ] Etiket/metadata (ID3/FLAC) okuma ve arama
- [ ] Ses seviyesi, çalma ilerleme çubuğu ve sürükleyerek ileri/geri sarma
- [ ] Karıştır/tekrarla modları

## Katkı
Her türlü katkıya açığız. Issue açabilir veya PR gönderebilirsiniz. Büyük değişiklikler için önce bir tartışma başlatmanız önerilir.

## Teşekkür
- [Fyne](https://fyne.io) — modern ve taşınabilir GUI
- [faiface/beep](https://github.com/faiface/beep) — basit ama güçlü ses çalma kütüphanesi

## Lisans
Bu proje GNU General Public License v3.0 (GPL‑3.0) ile lisanslanmıştır. Ayrıntılar için bkz. [LICENSE](LICENSE).
