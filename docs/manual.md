<div align="center">

<img src="https://github.com/openlibrecommunity/material/blob/master/olcrtc.png" width="250" height="250">

![License](https://img.shields.io/badge/license-WTFPL-0D1117?style=flat-square&logo=open-source-initiative&logoColor=green&labelColor=0D1117)
![Golang](https://img.shields.io/badge/-Golang-0D1117?style=flat-square&logo=go&logoColor=00A7D0)

</div>

# РњР°РЅСѓР°Р»СЊРЅР°СЏ СЃР±РѕСЂРєР°

> **Р’Р°Р¶РЅРѕ:** РћР±СЏР·Р°С‚РµР»СЊРЅРѕ РїСЂРѕРІРµСЂСЏР№С‚Рµ, РµСЃС‚СЊ Р»Рё СЃРµСЂРІРёСЃ РІРёРґРµРѕР·РІРѕРЅРєРѕРІ Сѓ РІР°СЃ РІ Р±РµР»С‹С… СЃРїРёСЃРєР°С…. Р•СЃР»Рё РµРіРѕ С‚Р°Рј РЅРµС‚ - РёСЃРїРѕР»СЊР·СѓР№С‚Рµ РґСЂСѓРіРѕР№. РЎРїРёСЃРѕРє РІСЃРµС… СЃРµСЂРІРёСЃРѕРІ РІ Р±РµР»С‹С… СЃРїРёСЃРєР°С… СЃРєРѕСЂРѕ Р±СѓРґРµС‚ РѕРїСѓР±Р»РёРєРѕРІР°РЅ.


Р­С‚РѕС‚ СЃРїРѕСЃРѕР± РґР»СЏ С‚РµС… РєС‚Рѕ С…РѕС‡РµС‚ СЃРѕР±СЂР°С‚СЊ Р±РёРЅР°СЂРЅРёРє СЂСѓРєР°РјРё Р±РµР· Docker/Podman.
РќСѓР¶РµРЅ Go 1.26+, mage, git.

---


### swap (РћР—РЈ)

Р•СЃР»Рё Сѓ РІР°СЃ РјРµРЅСЊС€Рµ 4Р“Р‘ РѕРїРµСЂР°С‚РёРІРЅРѕР№ РїР°РјСЏС‚Рё, СЃР±РѕСЂРєР° РјРѕР¶РµС‚ РІС‹Р»РµС‚Р°С‚СЊ. **РћР±СЏР·Р°С‚РµР»СЊРЅРѕ РІРєР»СЋС‡РёС‚Рµ SWAP**:

```bash
sudo fallocate -l 4G /swapfile && sudo chmod 600 /swapfile && sudo mkswap /swapfile && sudo swapon /swapfile
```


---

## Р§С‚Рѕ РЅСѓР¶РЅРѕ СѓСЃС‚Р°РЅРѕРІРёС‚СЊ

## РЁР°Рі 1: РЈСЃС‚Р°РЅРѕРІРёС‚СЊ git

```sh
apt install git       # Debian   / Ubuntu  / Mint
pacman -S git         # Arch    / CachyOS / Manjaro
dnf install git       # Fedora / RHEL   / CentOS
```

---

## РЁР°Рі 2: РЈСЃС‚Р°РЅРѕРІРёС‚СЊ Go 1.26+

### Arch / Fedora (РІСЃС‘ РїСЂРѕСЃС‚Рѕ)

```sh
pacman -S go    # Arch    / CachyOS / Manjaro
dnf install go  # Fedora / RHEL   / CentOS
```

### Debian / Ubuntu (СЃРёСЃС‚РµРјРЅС‹Р№ РїР°РєРµС‚ СѓСЃС‚Р°СЂРµРІС€РёР№)

РќР° Debian/Ubuntu РІ СЂРµРїРѕР·РёС‚РѕСЂРёРё РѕР±С‹С‡РЅРѕ Go 1.19.

РќР° Debian 13 Р»СѓС‡С€Рµ С‡РµСЂРµР· `testing` c `APT Pinning`, С‡С‚РѕР±С‹ РЅРµ Р·Р°СЃРѕСЂСЏС‚СЊ РћРЎ:

```sh
echo 'deb http://deb.debian.org/debian/ testing main non-free-firmware' | sudo tee /etc/apt/sources.list.d/testing.list

cat <<EOF | sudo tee /etc/apt/preferences.d/testing-pin
Package: *
Pin: release a=testing
Pin-Priority: 100
EOF

sudo apt update
sudo apt install -t testing golang-go

sudo update-alternatives --install /usr/bin/go go `which go` 10
sudo update-alternatives --install /usr/bin/gofmt gofmt `which gofmt` 10
```

РРЅР°С‡Рµ С‡РµСЂРµР· SDK:

```sh
apt install golang                         # СЃС‚Р°РІРёРј СЃС‚Р°СЂС‹Р№ go - РѕРЅ РЅСѓР¶РµРЅ С‚РѕР»СЊРєРѕ С‡С‚РѕР±С‹ СЃРєР°С‡Р°С‚СЊ РЅРѕРІС‹Р№
go install golang.org/dl/go1.26.0@latest   # СЃРєР°С‡РёРІР°РµРј СѓСЃС‚Р°РЅРѕРІС‰РёРє go1.26
~/go/bin/go1.26.0 download                 # СЃРєР°С‡РёРІР°РµРј СЃР°Рј go1.26
mv ~/go/bin/go1.26.0 /usr/local/bin/go     # Р·Р°РјРµРЅСЏРµРј СЃРёСЃС‚РµРјРЅС‹Р№ go
```

### РџСЂРѕРІРµСЂРєР°

```sh
go version
# go version go1.26.x linux/amd64
```

---

## РЁР°Рі 3: РЈСЃС‚Р°РЅРѕРІРёС‚СЊ mage

mage - СЃРёСЃС‚РµРјР° СЃР±РѕСЂРєРё РґР»СЏ Go-РїСЂРѕРµРєС‚РѕРІ, Р°РЅР°Р»РѕРі make.

```sh
go install github.com/magefile/mage@latest
```

Р”РѕР±Р°РІСЊ `~/go/bin` РІ PATH:

```sh
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

РџСЂРѕРІРµСЂРєР°:

```sh
mage --version
# mage vx.x.x
```

---

## РЁР°Рі 4: РЎРєР°С‡Р°С‚СЊ СЂРµРїРѕР·РёС‚РѕСЂРёР№

```sh
git clone https://github.com/fedorokss/olcrtc-clone
cd olcrtc
```


---

## РЁР°Рі 5: РЎРѕР±СЂР°С‚СЊ

```sh
mage build   # С‚РµРєСѓС‰Р°СЏ РїР»Р°С‚С„РѕСЂРјР°
mage cross   # РІСЃРµ РїР»Р°С‚С„РѕСЂРјС‹ СЃСЂР°Р·Сѓ (РµСЃР»Рё СЃРѕР±РёСЂР°РµС€СЊ РґР»СЏ РґСЂСѓРіРѕР№ РјР°С€РёРЅС‹)
```

Р РµР·СѓР»СЊС‚Р°С‚ РІ `build/`:

```
build/olcrtc-linux-amd64
```

---

## РЁР°Рі 6: РЎРіРµРЅРµСЂРёСЂРѕРІР°С‚СЊ РєР»СЋС‡ С€РёС„СЂРѕРІР°РЅРёСЏ

Р”РµР»Р°РµС‚СЃСЏ РѕРґРёРЅ СЂР°Р· РЅР° СЃРµСЂРІРµСЂРµ. РљР»СЋС‡ РґРѕР»Р¶РµРЅ СЃРѕРІРїР°РґР°С‚СЊ РЅР° СЃРµСЂРІРµСЂРµ Рё РєР»РёРµРЅС‚Рµ.

```sh
openssl rand -hex 32 
# d823fa01cb3e0609b67322f7cf984c4ee2e4ce2e294936fc24ef38c9e59f4799
```

РЎРѕС…СЂР°РЅРё РІС‹РІРѕРґ - РїРѕРЅР°РґРѕР±РёС‚СЃСЏ РїСЂРё Р·Р°РїСѓСЃРєРµ РєР»РёРµРЅС‚Р°.

---

## РЁР°Рі 7: Р—Р°РїСѓСЃС‚РёС‚СЊ СЃРµСЂРІРµСЂ

РќР° СЃРµСЂРІРµСЂРЅРѕР№ РјР°С€РёРЅРµ (VPS Рё С‚.Рґ.). РџРѕРґР±РµСЂРё РЅСѓР¶РЅСѓСЋ РєРѕРјР±РёРЅР°С†РёСЋ auth provider + transport РёР· РјР°С‚СЂРёС†С‹ РІ [settings.md](settings.md).

### jitsi + datachannel (СЂРµРєРѕРјРµРЅРґСѓРµС‚СЃСЏ)

РЎР°РјС‹Р№ РїСЂРѕСЃС‚РѕР№ СЃРїРѕСЃРѕР±: РёСЃРїРѕР»СЊР·СѓР№ Р»СЋР±РѕР№ self-hosted РёР»Рё РїСѓР±Р»РёС‡РЅС‹Р№ Jitsi Meet РёРЅСЃС‚Р°РЅСЃ. Р РµРіРёСЃС‚СЂР°С†РёСЏ РЅРµ РЅСѓР¶РЅР°, РёРјСЏ РєРѕРјРЅР°С‚С‹ РІС‹РґСѓРјС‹РІР°РµС‚СЃСЏ РЅР° Р»РµС‚Сѓ. Р”РѕСЃС‚СѓРїРЅС‹Рµ РїСѓР±Р»РёС‡РЅС‹Рµ СЃРµСЂРІРµСЂС‹: `meet1.arbitr.ru` Рё `meet.cryptopro.ru` - **РѕР±СЏР·Р°С‚РµР»СЊРЅРѕ РїСЂРѕРІРµСЂСЊ РІ Р±СЂР°СѓР·РµСЂРµ, РєР°РєРѕР№ РёР· РЅРёС… СЂР°Р±РѕС‚Р°РµС‚ РІ С‚РІРѕРµР№ СЃРµС‚Рё**, Рё РёСЃРїРѕР»СЊР·СѓР№ С‚РѕС‚, РєРѕС‚РѕСЂС‹Р№ РѕС‚РєСЂС‹РІР°РµС‚СЃСЏ. РўР°РєР¶Рµ РїРѕРґРѕР№РґС‘С‚ Р»СЋР±РѕР№ РґСЂСѓРіРѕР№ (`meet.jit.si`, СЃРІРѕР№ self-hosted Рё С‚.Рї.).

РЎРѕР·РґР°Р№ YAML РєРѕРЅС„РёРі:

```yaml
# server.yaml
mode: srv
auth:
  provider: jitsi
room:
  # РСЃРїРѕР»СЊР·СѓР№С‚Рµ meet1.arbitr.ru РёР»Рё meet.cryptopro.ru - С‚РѕС‚, С‡С‚Рѕ СЂР°Р±РѕС‚Р°РµС‚ РІ РІР°С€РµР№ СЃРµС‚Рё
  id: "https://meet1.arbitr.ru/myroom"
crypto:
  key: "d823fa01cb3e0609b67322f7cf984c4ee2e4ce2e294936fc24ef38c9e59f4799"
net:
  transport: datachannel
  dns: "8.8.8.8:53"
data: data
```

Р—Р°РїСѓСЃС‚Рё:

```sh
./build/olcrtc-linux-amd64 server.yaml
```

РЎРµСЂРІРµСЂ СЃР°Рј РїСЂРёСЃРѕРµРґРёРЅРёС‚СЃСЏ Рє РєРѕРјРЅР°С‚Рµ (РІ РєР°С‡РµСЃС‚РІРµ СѓС‡Р°СЃС‚РЅРёРєР° Р±РµР· РєР°РјРµСЂС‹/РјРёРєСЂРѕС„РѕРЅР°) Рё Р±СѓРґРµС‚ Р¶РґР°С‚СЊ, РїРѕРєР° РєР»РёРµРЅС‚ С‚РѕР¶Рµ Р·Р°Р№РґС‘С‚. Р‘РµР· РІС‚РѕСЂРѕРіРѕ СѓС‡Р°СЃС‚РЅРёРєР° Jicofo РЅРµ РІС‹РґР°С‘С‚ session-initiate - СЌС‚Рѕ РѕСЃРѕР±РµРЅРЅРѕСЃС‚СЊ Jitsi.

### wbstream + vp8channel (Р°Р»СЊС‚РµСЂРЅР°С‚РёРІР°)

РЎРѕР·РґР°Р№ СЂСѓРјСѓ С‡РµСЂРµР· СЃР°Р№С‚ [wbstream](https://stream.wb.ru) Рё РІСЃС‚Р°РІСЊ РµС‘ ID РІ `room.id`.

`wbstream + datachannel` **РЅРµ СЂР°Р±РѕС‚Р°РµС‚** РІ РѕР±С‹С‡РЅРѕРј guest flow - WB Stream РІС‹РґР°С‘С‚ С‚РѕРєРµРЅС‹ СЃ `canPublishData=false`, Рё DC РЅРµ РјР°СЂС€СЂСѓС‚РёР·РёСЂСѓРµС‚ РґР°РЅРЅС‹Рµ. Р”Р»СЏ РѕР±С‹С‡РЅРѕРіРѕ РёСЃРїРѕР»СЊР·РѕРІР°РЅРёСЏ РІС‹Р±РёСЂР°Р№ `vp8channel`.

РЎРѕР·РґР°Р№ YAML РєРѕРЅС„РёРі:

```yaml
# server.yaml
mode: srv
auth:
  provider: wbstream
room:
  id: "<room-id-СЃРѕ-stream.wb.ru>"
crypto:
  key: "d823fa01cb3e0609b67322f7cf984c4ee2e4ce2e294936fc24ef38c9e59f4799"
net:
  transport: vp8channel
  dns: "8.8.8.8:53"
data: data
```

Р—Р°РїСѓСЃС‚Рё:

```sh
./build/olcrtc-linux-amd64 server.yaml
```

Room ID РЅСѓР¶РЅРѕ РїРµСЂРµРґР°С‚СЊ РєР»РёРµРЅС‚Сѓ.

### Р”РѕР±Р°РІРёС‚СЊ РѕС‚Р»Р°РґРєСѓ

Р”РѕР±Р°РІСЊ `debug: true` РІ YAML РєРѕРЅС„РёРі - СѓРІРёРґРёС€СЊ РєР°Р¶РґРѕРµ СЃРѕРµРґРёРЅРµРЅРёРµ:

```
2026/05/03 08:05:23 Connecting link via direct/vp8channel/wbstream...
2026/05/03 08:05:25 wbstream publisher state: connected
2026/05/03 08:05:27 Link connected
2026/05/03 08:05:43 sid=3 connect icanhazip.com:443
2026/05/03 08:05:43 sid=3 connected icanhazip.com
```

---

## РЁР°Рі 8: Р—Р°РїСѓСЃС‚РёС‚СЊ РєР»РёРµРЅС‚

РќР° СЃРІРѕРµР№ РјР°С€РёРЅРµ. `auth.provider`, `net.transport`, `room.id` Рё `crypto.key` РґРѕР»Р¶РЅС‹ СЃРѕРІРїР°РґР°С‚СЊ СЃ СЃРµСЂРІРµСЂРѕРј.

### jitsi + datachannel (СЂРµРєРѕРјРµРЅРґСѓРµС‚СЃСЏ)

```yaml
# client.yaml
mode: cnc
auth:
  provider: jitsi
room:
  # РСЃРїРѕР»СЊР·СѓР№С‚Рµ meet1.arbitr.ru РёР»Рё meet.cryptopro.ru - С‚РѕС‚, С‡С‚Рѕ СЂР°Р±РѕС‚Р°РµС‚ РІ РІР°С€РµР№ СЃРµС‚Рё
  id: "https://meet1.arbitr.ru/myroom"
crypto:
  key: "<hex-key-С‚Р°РєРѕР№-Р¶Рµ-РєР°Рє-РЅР°-СЃРµСЂРІРµСЂРµ>"
net:
  transport: datachannel
  dns: "8.8.8.8:53"
socks:
  host: "127.0.0.1"
  port: 8808
data: data
```

```sh
./build/olcrtc-linux-amd64 client.yaml
```

РџРѕСЃР»Рµ Р·Р°РїСѓСЃРєР° SOCKS5 Р±СѓРґРµС‚ СЃР»СѓС€Р°С‚СЊ РЅР° `127.0.0.1:8808`. РСЃРїРѕР»СЊР·СѓР№ Р»СЋР±РѕР№ РєР»РёРµРЅС‚ СЃ РїРѕРґРґРµСЂР¶РєРѕР№ SOCKS5 (`curl --socks5 127.0.0.1:8808 ...`, Р±СЂР°СѓР·РµСЂ СЃ РїРµСЂРµРєР»СЋС‡Р°С‚РµР»РµРј РїСЂРѕРєСЃРё Рё С‚.Рї.).

### wbstream + vp8channel (Р°Р»СЊС‚РµСЂРЅР°С‚РёРІР°)

```yaml
# client.yaml
mode: cnc
auth:
  provider: wbstream
room:
  id: "<room-id>"
crypto:
  key: "<hex-key>"
net:
  transport: vp8channel
  dns: "8.8.8.8:53"
socks:
  host: "127.0.0.1"
  port: 8808
data: data
```

```sh
./build/olcrtc-linux-amd64 client.yaml
```

РџРѕСЃР»Рµ СЃС‚Р°СЂС‚Р° РІ Р»РѕРіР°С… РїРѕСЏРІРёС‚СЃСЏ:

```
SOCKS5 server listening on 127.0.0.1:8808
```

Р•СЃР»Рё РЅСѓР¶РЅРѕ Р·Р°С‰РёС‚РёС‚СЊ РїСЂРѕРєСЃРё Р»РѕРіРёРЅРѕРј Рё РїР°СЂРѕР»РµРј (РЅР°РїСЂРёРјРµСЂ РЅР° РјР°С€РёРЅРµ СЃ РЅРµСЃРєРѕР»СЊРєРёРјРё РїРѕР»СЊР·РѕРІР°С‚РµР»СЏРјРё), РґРѕР±Р°РІСЊ `socks.user` Рё `socks.pass` РІ РєРѕРЅС„РёРі:

```yaml
# client.yaml
mode: cnc
auth:
  provider: wbstream
room:
  id: "<room-id>"
crypto:
  key: "<hex-key>"
net:
  transport: vp8channel
  dns: "8.8.8.8:53"
socks:
  host: "127.0.0.1"
  port: 8808
  user: myuser
  pass: mypass
data: data
```

Р‘РµР· СЌС‚РёС… РїРѕР»РµР№ Р°СѓС‚РµРЅС‚РёС„РёРєР°С†РёСЏ РѕС‚РєР»СЋС‡РµРЅР° - РїРѕРІРµРґРµРЅРёРµ РїСЂРµР¶РЅРµРµ.

---

## РЁР°Рі 9: РџСЂРѕРІРµСЂРёС‚СЊ

```sh
curl --socks5-hostname 127.0.0.1:8808 https://icanhazip.com
```

Р”РѕР»Р¶РµРЅ РІРµСЂРЅСѓС‚СЊ IP СЃРµСЂРІРµСЂР°.


---

## Р’СЃРµ mage С‚Р°СЂРіРµС‚С‹

### РЎР±РѕСЂРєР°
```sh
mage build    # СЃРѕР±СЂР°С‚СЊ РґР»СЏ С‚РµРєСѓС‰РµР№ РїР»Р°С‚С„РѕСЂРјС‹
mage cross    # СЃРѕР±СЂР°С‚СЊ РґР»СЏ РІСЃРµС… РїР»Р°С‚С„РѕСЂРј
mage mobile   # СЃРѕР±СЂР°С‚СЊ Android AAR
mage docker   # СЃРѕР±СЂР°С‚СЊ РѕР±СЂР°Р· С‡РµСЂРµР· docker
mage podman   # СЃРѕР±СЂР°С‚СЊ РѕР±СЂР°Р· С‡РµСЂРµР· podman
mage clean    # СѓРґР°Р»РёС‚СЊ build/
```

### РљР°С‡РµСЃС‚РІРѕ
```sh
mage vet      # go vet
mage lint     # golangci-lint
mage tidy     # go mod tidy && go mod verify
mage deps     # go mod download
```

### РўРµСЃС‚С‹
```sh
mage test       # СЋРЅРёС‚С‹ РІ -short, Р±С‹СЃС‚СЂРѕ
mage testFull   # РІСЃРµ СЋРЅРёС‚С‹ + Р»РѕРєР°Р»СЊРЅС‹Рµ e2e СЃ -race
mage e2e        # smoke-РјР°С‚СЂРёС†Р° РїСЂРѕС‚РёРІ СЂРµР°Р»СЊРЅС‹С… РїСЂРѕРІР°Р№РґРµСЂРѕРІ
mage stress     # stress-РјР°С‚СЂРёС†Р° (~6 С‡)
mage soak       # СЂРµР°Р»СЊРЅС‹Р№ soak (С‡Р°СЃР°РјРё)
mage localSoak  # in-memory soak (Р±РµР· СЃРµС‚Рё)
```

### РџР°Р№РїР»Р°Р№РЅС‹
```sh
mage check       # build + vet + lint + testFull (РїРµСЂРµРґ РєРѕРјРјРёС‚РѕРј)
mage all         # check + e2e (РїРµСЂРµРґ РјРµСЂРґР¶РµРј PR)
mage nightly     # all + stress (РЅРѕС‡РЅРѕР№ CI, ~6 С‡)
mage everything  # nightly + soak + localSoak (РїРѕР»РЅР°СЏ РІР°Р»РёРґР°С†РёСЏ, 12+ С‡)
```

### РџСЂРѕС‡РµРµ
```sh
mage help     # СЃРїРёСЃРѕРє С‚Р°СЂРіРµС‚РѕРІ РІ СЃС‚Р°РЅРґР°СЂС‚РЅРѕРј СЃС‚РёР»Рµ mage
mage -l       # С‚Рѕ Р¶Рµ С‡С‚Рѕ mage help
mage         # Р±РµР· Р°СЂРіСѓРјРµРЅС‚РѕРІ = mage help
```

РўРѕРЅРєР°СЏ РЅР°СЃС‚СЂРѕР№РєР° РїСЂРѕРіРѕРЅР° С‚РµСЃС‚РѕРІ С‡РµСЂРµР· РїРµСЂРµРјРµРЅРЅС‹Рµ РѕРєСЂСѓР¶РµРЅРёСЏ:

```sh
# РѕРґРёРЅРѕС‡РЅС‹Р№ РєРµР№СЃ stress
E2E_CARRIERS=telemost E2E_TRANSPORTS=videochannel \
    STRESS_BULK_DURATION=0 STRESS_ECHO_DURATION=0 \
    STRESS_CASE_TIMEOUT=2m STRESS_TIMEOUT=3m mage stress

# soak С‚РѕР»СЊРєРѕ jitsi РЅР° 30 РјРёРЅСѓС‚
SOAK_CARRIERS=jitsi SOAK_DURATION=30m mage soak
```

РџРѕР»РЅС‹Р№ СЃРїРёСЃРѕРє РїРµСЂРµРјРµРЅРЅС‹С…:
- `E2E_CARRIERS`, `E2E_TRANSPORTS`, `E2E_TIMEOUT`, `E2E_STRESS`, `E2E_STRESS_DURATION`
- `STRESS_BULK_DURATION`, `STRESS_ECHO_DURATION`, `STRESS_CASE_TIMEOUT`, `STRESS_TIMEOUT`
- `SOAK_CARRIERS`, `SOAK_TRANSPORTS`, `SOAK_DURATION`, `SOAK_CHAOS`
- `DOCKER_TAG`

---

## РќРµСЃРєРѕР»СЊРєРѕ РёРЅСЃС‚Р°РЅСЃРѕРІ РЅР° РѕРґРЅРѕРј СЃРµСЂРІРµСЂРµ

РњРѕР¶РЅРѕ Р·Р°РїСѓСЃС‚РёС‚СЊ РЅРµСЃРєРѕР»СЊРєРѕ СЃРµСЂРІРµСЂРѕРІ olcrtc РЅР° РѕРґРЅРѕР№ РјР°С€РёРЅРµ - РєР°Р¶РґС‹Р№ СЃРѕ СЃРІРѕРёРј РєРѕРЅС„РёРіРѕРј (СЂР°Р·РЅС‹Рµ РїСЂРѕРІР°Р№РґРµСЂС‹, РєРѕРјРЅР°С‚С‹, С‚СЂР°РЅСЃРїРѕСЂС‚С‹). Р”Р»СЏ СЌС‚РѕРіРѕ СЃРѕР·РґР°Р№ РѕС‚РґРµР»СЊРЅС‹Р№ YAML-С„Р°Р№Р» РґР»СЏ РєР°Р¶РґРѕРіРѕ РёРЅСЃС‚Р°РЅСЃР° Рё Р·Р°РїСѓСЃС‚Рё РєР°Р¶РґС‹Р№ РІ РѕС‚РґРµР»СЊРЅРѕРј РїСЂРѕС†РµСЃСЃРµ.

### РџСЂРёРјРµСЂ: РґРІР° СЃРµСЂРІРµСЂР°

```yaml
# server-jitsi.yaml
mode: srv
auth:
  provider: jitsi
room:
  id: "https://meet1.arbitr.ru/room1"
crypto:
  key: "aaaa...1111"
net:
  transport: datachannel
  dns: "8.8.8.8:53"
data: data
```

```yaml
# server-wbstream.yaml
mode: srv
auth:
  provider: wbstream
room:
  id: "<room-id>"
crypto:
  key: "bbbb...2222"
net:
  transport: vp8channel
  dns: "8.8.8.8:53"
data: data
```

Р—Р°РїСѓСЃС‚Рё РєР°Р¶РґС‹Р№ РІ РѕС‚РґРµР»СЊРЅРѕРј С‚РµСЂРјРёРЅР°Р»Рµ (РёР»Рё С‡РµСЂРµР· `tmux` / `screen` / `systemd`):

```sh
./build/olcrtc-linux-amd64 server-jitsi.yaml
./build/olcrtc-linux-amd64 server-wbstream.yaml
```

### РљР»РёРµРЅС‚С‹

РќР° РєР»РёРµРЅС‚СЃРєРѕР№ РјР°С€РёРЅРµ - РїРѕ РѕРґРЅРѕРјСѓ РєРѕРЅС„РёРіСѓ РЅР° РєР°Р¶РґС‹Р№ СЃРµСЂРІРµСЂ, СЃ **СЂР°Р·РЅС‹РјРё SOCKS5 РїРѕСЂС‚Р°РјРё**:

```yaml
# client-jitsi.yaml
mode: cnc
auth:
  provider: jitsi
room:
  id: "https://meet1.arbitr.ru/room1"
crypto:
  key: "aaaa...1111"
net:
  transport: datachannel
  dns: "8.8.8.8:53"
socks:
  host: "127.0.0.1"
  port: 8808
data: data
```

```yaml
# client-wbstream.yaml
mode: cnc
auth:
  provider: wbstream
room:
  id: "<room-id>"
crypto:
  key: "bbbb...2222"
net:
  transport: vp8channel
  dns: "8.8.8.8:53"
socks:
  host: "127.0.0.1"
  port: 8809
data: data
```

```sh
./build/olcrtc-linux-amd64 client-jitsi.yaml      # SOCKS5 РЅР° :8808
./build/olcrtc-linux-amd64 client-wbstream.yaml    # SOCKS5 РЅР° :8809
```

РџРµСЂРµРєР»СЋС‡РµРЅРёРµ РјРµР¶РґСѓ РёРЅСЃС‚Р°РЅСЃР°РјРё РІ olcbox - РїСЂРѕСЃС‚Рѕ РІС‹Р±РёСЂР°РµС€СЊ РЅСѓР¶РЅС‹Р№ SOCKS5 РїРѕСЂС‚.

---

РСЃРїРѕР»СЊР·СѓРµС€СЊ СЃРєСЂРёРїС‚С‹ РІРјРµСЃС‚Рѕ СЂСѓС‡РЅРѕР№ СЃР±РѕСЂРєРё? -> [Р‘С‹СЃС‚СЂС‹Р№ СЃС‚Р°СЂС‚](fast.md)

Р’СЃРµ РЅР°СЃС‚СЂРѕР№РєРё Рё РјР°С‚СЂРёС†Р° СЃРѕРІРјРµСЃС‚РёРјРѕСЃС‚Рё -> [settings.md](settings.md)
