# webrt-img-resize

> Utilitar CLI în Go (1.26+), un singur binar, fără dependențe de sistem.
> Aduce toate imaginile dintr-un folder la aceleași dimensiuni maxime.

Originalele nu sunt modificate niciodată: rezultatul se scrie într-un alt folder
(implicit `<sursă>/resized`).

## Cuprins

1. [Instalare / compilare](#1-instalare--compilare)
2. [Utilizare rapidă](#2-utilizare-rapidă)
3. [Moduri](#3-moduri)
4. [Opțiuni](#4-opțiuni)
5. [Formate](#5-formate)
6. [Comportament și limitări](#6-comportament-și-limitări)
7. [Dezvoltare](#7-dezvoltare)
8. [Referință rapidă fișiere](#8-referință-rapidă-fișiere)

## 1. Instalare / compilare

Cerință pentru compilare: Go 1.26 sau mai nou (impus de `golang.org/x/image`) și `make`.
Binarul rezultat e static, deci pe mașina unde rulează nu e nevoie de Go.

### Instalare globală (recomandat)

```bash
make install
```

Comanda compilează ca utilizatorul curent, apoi copiază binarul în
`/usr/local/bin/webrt-img-resize`. Pentru copiere cere parola de `sudo`, fiindcă folderul
aparține lui root. După aceea comanda merge din orice folder:

```bash
cd ~/Poze && webrt-img-resize -maxw 1200 -maxh 1200
```

Actualizare după modificări în cod: rulezi din nou `make install`.
Dezinstalare: `make uninstall`.

Varianta fără sudo, doar pentru utilizatorul curent (în `~/.local/bin`):

```bash
make install PREFIX=~/.local
make uninstall PREFIX=~/.local
```

> Nu ține ambele variante instalate: `~/.local/bin` e de obicei înaintea lui
> `/usr/local/bin` în `PATH`, iar o copie veche de acolo ar ascunde versiunea nouă.
> Verifică ce se rulează cu `which webrt-img-resize`.
>
> Nici `go install .` nu e recomandat: pune binarul în `~/go/bin`, care de obicei **nu**
> e în `PATH`.

### Doar compilare locală

```bash
make build        # → ./webrt-img-resize
```

### Binare pentru alte sisteme

```bash
make release      # → dist/
```

| Fișier din `dist/`                     | Sistem                         |
|----------------------------------------|--------------------------------|
| `webrt-img-resize-linux-amd64`         | Linux x86_64                   |
| `webrt-img-resize-linux-arm64`         | Linux ARM (Raspberry Pi 4/5, servere ARM) |
| `webrt-img-resize-macos-arm64`         | macOS Apple Silicon (M1+)      |
| `webrt-img-resize-macos-amd64`         | macOS Intel                    |
| `webrt-img-resize-windows-amd64.exe`   | Windows 64-bit                 |

Pe macOS, la prima rulare a unui binar descărcat: `xattr -d com.apple.quarantine <fișier>`.
Pe Windows se rulează din `cmd`/PowerShell: `webrt-img-resize-windows-amd64.exe -in C:\Poze -maxw 1200`.

## 2. Utilizare rapidă

```bash
# toate pozele din ./poze încadrate în max 1200×1200 → ./poze/resized
webrt-img-resize -in ./poze -maxw 1200 -maxh 1200

# thumbnail-uri pătrate de exact 400×400, decupate centrat, toate în JPEG
webrt-img-resize -in ./poze -out ./thumbs -mode fill -maxw 400 -maxh 400 -format jpg -q 80

# toate exact 800×600, cu margini albe, fără a mări pozele mici
webrt-img-resize -in ./produse -mode pad -maxw 800 -maxh 600 -noup

# doar lățime maximă, înălțimea rezultă din proporții
webrt-img-resize -in ./poze -maxw 1600
```

Exemplu de output:

```
6 imagini → /home/user/poze/resized  [mode=fit, 1000x1000]
✓ a.jpg  4000x3000 → a.jpg 1000x750
✓ c.gif  300x200 → c-gif.png 1000x667
✗ broken.jpg: decodare: image: unknown format

Gata: 5 reușite, 1 erori.
```

## 3. Moduri

| Mod    | Ce face                                                             | Dimensiune finală        | Cere           |
|--------|---------------------------------------------------------------------|--------------------------|----------------|
| `fit`  | încadrează în `maxw×maxh` păstrând proporțiile (implicit)           | variabilă, ≤ `maxw×maxh` | `-maxw` și/sau `-maxh` |
| `pad`  | ca `fit`, apoi centrează pe un fundal de culoarea `-bg`             | exact `maxw×maxh`        | ambele         |
| `fill` | umple complet `maxw×maxh` și decupează centrat ce iese în afară     | exact `maxw×maxh`        | ambele         |

## 4. Opțiuni

| Flag       | Implicit          | Descriere |
|------------|-------------------|-----------|
| `-in`      | `.`               | folderul sursă (doar primul nivel, fără subfoldere) |
| `-out`     | `<in>/resized`    | folderul destinație; nu poate fi același cu sursa |
| `-maxw`    | `0`               | lățimea maximă în px |
| `-maxh`    | `0`               | înălțimea maximă în px |
| `-mode`    | `fit`             | `fit` \| `pad` \| `fill` |
| `-format`  | (păstrează)       | forțează ieșirea: `jpg` \| `png` |
| `-q`       | `85`              | calitate JPEG, 1–100 |
| `-noup`    | `false`           | nu mări imaginile mai mici decât limita (`fit`/`pad`) |
| `-bg`      | `ffffff`          | fundal pentru `pad` și pentru JPEG cu transparență: hex (`#000000`) sau `transparent` |
| `-workers` | nr. de CPU-uri    | câte imagini se procesează în paralel |

`webrt-img-resize -h` afișează lista completă.

## 5. Formate

| Intrare              | Ieșire implicită | Observații |
|----------------------|------------------|------------|
| `.jpg` / `.jpeg`     | aceeași extensie | orientarea EXIF (poze de pe telefon) este aplicată automat |
| `.png`               | `.png`           | transparența se păstrează |
| `.gif`               | `.png`           | doar primul cadru dacă e animat |
| `.webp`              | `.png`           | Go nu are encoder WebP, deci nu se poate scrie înapoi WebP |

La ieșire JPEG, transparența se aplatizează pe culoarea `-bg`. Dacă `-bg transparent`,
se folosește alb.

## 6. Comportament și limitări

- **Fără `-noup`, imaginile mici sunt mărite** până la limită (în `fit` și `pad`). Pentru
  galerii cu poze de mărimi amestecate, folosește de regulă `-noup`.
- **Coliziuni de nume:** dacă două surse ar produce același fișier (ex. `poza.gif` și
  `poza.png` → `poza.png`), ambele primesc ca sufix extensia originală: `poza-gif.png`,
  `poza-png.png`. Fișierele fără coliziune își păstrează numele.
- Fișierele deja existente în destinație sunt **suprascrise** fără avertisment.
- Metadatele (EXIF, profil ICC) nu sunt copiate în fișierele rezultate.
- Scalarea folosește Catmull-Rom (calitate bună, comparabilă cu bicubic).
- Un fișier corupt nu oprește procesarea: apare cu `✗` și e numărat la erori.
- Memorie: fiecare worker ține în RAM o imagine decodată. Pentru foarte multe poze de
  50+ MP pe o mașină cu puțin RAM, redu `-workers`.
- Codul de ieșire este `1` doar pentru erori de parametri sau de folder. Erorile
  individuale pe imagini nu schimbă codul de ieșire.

## 7. Dezvoltare

```bash
make test       # go vet + go test
gofmt -l .      # trebuie să nu afișeze nimic
make clean      # șterge binarul local și dist/
```

Singura dependență externă este `golang.org/x/image`, pentru `draw` (scalare) și decodarea WebP.

## 8. Referință rapidă fișiere

```
webrt-img-resize/
├── webrt-img-resize.go       # tot programul: flag-uri, worker pool, fit/pad/fill, salvare, EXIF
├── webrt-img-resize_test.go  # teste pentru dimensiuni, culori, extensii, coliziuni, rotire EXIF
├── go.mod / go.sum           # modulul și dependența golang.org/x/image
├── Makefile                  # build / install / uninstall / test / release / clean
├── .gitignore                # binarul compilat, dist/, .idea/
└── README.md
```
