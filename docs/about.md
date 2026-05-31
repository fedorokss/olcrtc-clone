<div align="center">

<img src="https://github.com/openlibrecommunity/material/blob/master/olcrtc.png" width="250" height="250">

![License](https://img.shields.io/badge/license-WTFPL-0D1117?style=flat-square&logo=open-source-initiative&logoColor=green&labelColor=0D1117)
![Golang](https://img.shields.io/badge/-Golang-0D1117?style=flat-square&logo=go&logoColor=00A7D0)

</div>



# olcRTC - РѕР±С‰РµРµ РѕРїРёСЃР°РЅРёРµ

`olcRTC` (OpenLibreCommunity RTC) - Р·Р°С€РёС„СЂРѕРІР°РЅРЅС‹Р№ TCP-over-WebRTC С‚СѓРЅРЅРµР»СЊ. РћРЅ РјР°СЃРєРёСЂСѓРµС‚ С‚СЂР°С„РёРє РїРѕРґ РѕР±С‹С‡РЅРѕРµ СѓС‡Р°СЃС‚РёРµ РІ WebRTC/SFU-СЃРµСЂРІРёСЃРµ: Jitsi Meet, Yandex Telemost РёР»Рё WbStream.

РџСЂРѕРµРєС‚: [github.com/fedorokss/olcrtc-clone](https://github.com/fedorokss/olcrtc-clone)  
Р›РёС†РµРЅР·РёСЏ: WTFPL  
РЎС‚Р°С‚СѓСЃ: **Beta**

## Р—Р°С‡РµРј СЌС‚Рѕ РЅСѓР¶РЅРѕ

Р’ СЃС†РµРЅР°СЂРёСЏС…, РіРґРµ РїСЂСЏРјРѕР№ РґРѕСЃС‚СѓРї Рє РїСЂРѕРёР·РІРѕР»СЊРЅРѕРјСѓ VPS / IP Р·Р°Р±Р»РѕРєРёСЂРѕРІР°РЅ, РїСЂРёС…РѕРґРёС‚СЃСЏ РїРµСЂРµРЅРѕСЃРёС‚СЊ С‚СЂР°С„РёРє С‡РµСЂРµР· СЃРµСЂРІРёСЃС‹, РєРѕС‚РѕСЂС‹Рµ СѓР¶Рµ РґРѕСЃС‚СѓРїРЅС‹ Сѓ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ. Р”Р»СЏ РІРЅРµС€РЅРµРіРѕ РЅР°Р±Р»СЋРґР°С‚РµР»СЏ СЃРѕРµРґРёРЅРµРЅРёРµ РІС‹РіР»СЏРґРёС‚ РєР°Рє РѕР±С‹С‡РЅС‹Р№ WebRTC-Р·РІРѕРЅРѕРє РїРѕ СЂР°Р·СЂРµС€РµРЅРЅРѕРјСѓ IP СЃРµСЂРІРёСЃР°, Р° РїРѕР»РµР·РЅР°СЏ РЅР°РіСЂСѓР·РєР° РІРЅСѓС‚СЂРё РґРѕРїРѕР»РЅРёС‚РµР»СЊРЅРѕ С€РёС„СЂСѓРµС‚СЃСЏ РѕР±С‰РёРј РєР»СЋС‡РѕРј `crypto.key`.

> **Р’Р°Р¶РЅРѕ:** РћР±СЏР·Р°С‚РµР»СЊРЅРѕ РїСЂРѕРІРµСЂСЏР№С‚Рµ, РµСЃС‚СЊ Р»Рё СЃРµСЂРІРёСЃ РІРёРґРµРѕР·РІРѕРЅРєРѕРІ Сѓ РІР°СЃ РІ Р±РµР»С‹С… СЃРїРёСЃРєР°С…. Р•СЃР»Рё РµРіРѕ С‚Р°Рј РЅРµС‚ - РёСЃРїРѕР»СЊР·СѓР№С‚Рµ РґСЂСѓРіРѕР№. РЎРїРёСЃРѕРє РІСЃРµС… СЃРµСЂРІРёСЃРѕРІ РІ Р±РµР»С‹С… СЃРїРёСЃРєР°С… СЃРєРѕСЂРѕ Р±СѓРґРµС‚ РѕРїСѓР±Р»РёРєРѕРІР°РЅ.

Р‘Р°Р·РѕРІР°СЏ СЃС…РµРјР°:

```text
РїСЂРёР»РѕР¶РµРЅРёРµ
  -> SOCKS5 127.0.0.1:8808
   -> olcrtc cnc
    -> WebRTC/SFU СЃРµСЂРІРёСЃ
     -> olcrtc srv
       -> РёРЅС‚РµСЂРЅРµС‚
```

## РљР°Рє СЌС‚Рѕ СЂР°Р±РѕС‚Р°РµС‚

РљР»РёРµРЅС‚СЃРєРёР№ СЂРµР¶РёРј `cnc` РїРѕРґРЅРёРјР°РµС‚ Р»РѕРєР°Р»СЊРЅС‹Р№ SOCKS5. Р‘СЂР°СѓР·РµСЂ, curl, sing-box, olcbox РёР»Рё РґСЂСѓРіРѕРµ РїСЂРёР»РѕР¶РµРЅРёРµ РїРѕРґРєР»СЋС‡Р°РµС‚СЃСЏ Рє РЅРµРјСѓ РєР°Рє Рє РѕР±С‹С‡РЅРѕРјСѓ proxy.

РЎРµСЂРІРµСЂРЅС‹Р№ СЂРµР¶РёРј `srv` РїРѕРґРєР»СЋС‡Р°РµС‚СЃСЏ Рє С‚РѕР№ Р¶Рµ РєРѕРјРЅР°С‚Рµ/СЃРµСЃСЃРёРё, РїСЂРёРЅРёРјР°РµС‚ Р·Р°С€РёС„СЂРѕРІР°РЅРЅС‹Р№ smux stream Рё РѕС‚ СЃРІРѕРµРіРѕ РёРјРµРЅРё РѕС‚РєСЂС‹РІР°РµС‚ TCP-СЃРѕРµРґРёРЅРµРЅРёСЏ Рє С†РµР»РµРІС‹Рј Р°РґСЂРµСЃР°Рј.

Р’РЅСѓС‚СЂРё С‚СѓРЅРЅРµР»СЏ:

```text
SOCKS CONNECT
  -> smux stream
   -> XChaCha20-Poly1305
    -> transport
     -> engine
      -> WebRTC/SFU
```

## Р РµР¶РёРјС‹

| Р РµР¶РёРј | РќР°Р·РЅР°С‡РµРЅРёРµ |
|---|---|
| `srv` | СЃРµСЂРІРµСЂРЅР°СЏ СЃС‚РѕСЂРѕРЅР°, РїСЂРёРЅРёРјР°РµС‚ tunnel streams Рё РґРµР»Р°РµС‚ TCP dial Рє С†РµР»СЏРј |
| `cnc` | РєР»РёРµРЅС‚СЃРєР°СЏ СЃС‚РѕСЂРѕРЅР°, СЃР»СѓС€Р°РµС‚ Р»РѕРєР°Р»СЊРЅС‹Р№ SOCKS5 |
| `gen` | СЃРѕР·РґР°С‘С‚ Room ID РґР»СЏ РїСЂРѕРІР°Р№РґРµСЂРѕРІ, РєРѕС‚РѕСЂС‹Рµ СѓРјРµСЋС‚ СЃРѕР·РґР°РІР°С‚СЊ РєРѕРјРЅР°С‚С‹ |

CLI РїСЂРёРЅРёРјР°РµС‚ РѕРґРёРЅ YAML-С„Р°Р№Р»:

```bash
olcrtc server.yaml
olcrtc client.yaml
```

## Auth Providers

`auth.provider` РІС‹Р±РёСЂР°РµС‚ СЃРµСЂРІРёСЃ Рё СЃРїРѕСЃРѕР± РїРѕР»СѓС‡РµРЅРёСЏ credentials.

| Provider | Engine | РљРѕРјРјРµРЅС‚Р°СЂРёР№ |
|---|---|---|
| `jitsi` | `jitsi` | URL РєРѕРјРЅР°С‚С‹ Jitsi (`meet1.arbitr.ru` РёР»Рё `meet.cryptopro.ru`), Р±РµР· РѕС‚РґРµР»СЊРЅРѕР№ СЂРµРіРёСЃС‚СЂР°С†РёРё |
| `telemost` | `goolom` | credentials С‡РµСЂРµР· Yandex Telemost API, СЃ РѕС‚РґРµР»СЊРЅРѕР№ СЂРµРіРёСЃС‚СЂР°С†РёРµР№ |
| `wbstream` | `livekit` | credentials С‡РµСЂРµР· WbBStream API, СЃ РѕС‚РґРµР»СЊРЅРѕР№ СЂРµРіРёСЃС‚СЂР°С†РёРµР№ |
| `none` | Р·Р°РґР°С‘С‚СЃСЏ РІ `engine.name` | РїСЂСЏРјРѕР№ engine-СЂРµР¶РёРј СЃ `engine.url` Рё `engine.token`, СЃ РѕС‚РґРµР»СЊРЅРѕР№ СЂРµРіРёСЃС‚СЂР°С†РёРµР№ |

РўРµСЂРјРёРЅ `carrier` РµС‰С‘ РІСЃС‚СЂРµС‡Р°РµС‚СЃСЏ РІРѕ РІРЅСѓС‚СЂРµРЅРЅРµРј API Рё Р»РѕРіР°С… РєР°Рє РёСЃС‚РѕСЂРёС‡РµСЃРєРѕРµ РёРјСЏ РґР»СЏ РІС‹Р±СЂР°РЅРЅРѕРіРѕ auth/provider РїСѓС‚Рё. Р’ YAML Р°РєС‚СѓР°Р»СЊРЅРѕРµ РїРѕР»Рµ - `auth.provider`.

## Engines

`engine` - РЅРёР·РєРѕСѓСЂРѕРІРЅРµРІС‹Р№ РїСЂРѕС‚РѕРєРѕР» РєРѕРЅРєСЂРµС‚РЅРѕРіРѕ SFU/signaling:

| Engine | РџР°РєРµС‚ | Р’РѕР·РјРѕР¶РЅРѕСЃС‚Рё |
|---|---|---|
| `livekit` | `internal/engine/livekit` | data packets/video tracks/LiveKit SDK |
| `goolom` | `internal/engine/goolom` | Telemost/Goolom signaling, publisher/subscriber PeerConnection |
| `jitsi` | `internal/engine/jitsi` | Jitsi MUC/Jingle/colibri-ws, datachannel/best-effort video |

`internal/engine/builtin` СЃРІСЏР·С‹РІР°РµС‚ `auth.provider` СЃ РЅСѓР¶РЅС‹Рј engine. РћС‚РґРµР»СЊРЅРѕРіРѕ РїР°РєРµС‚Р° `internal/carrier` РІ С‚РµРєСѓС‰РµРј РїСЂРѕРµРєС‚Рµ РЅРµС‚.

## Transports

`net.transport` РѕРїСЂРµРґРµР»СЏРµС‚, РєР°Рє tunnel bytes РїРѕРјРµС‰Р°СЋС‚СЃСЏ РІ WebRTC primitive.

| Transport | РљР°Рє РїРµСЂРµРґР°С‘С‚ РґР°РЅРЅС‹Рµ | РћСЃРЅРѕРІРЅРѕР№ СЃС†РµРЅР°СЂРёР№ |
|---|---|---|
| `datachannel` | РЅР°С‚РёРІРЅС‹Р№ byte/data path engine | СЃР°РјС‹Р№ РїСЂРѕСЃС‚РѕР№ Рё Р±С‹СЃС‚СЂС‹Р№ РїСѓС‚СЊ, СЃС‚Р°Р±РёР»СЊРЅРѕ СЃ Jitsi |
| `vp8channel` | KCP РїРѕРІРµСЂС… VP8-like video frames | РѕСЃРЅРѕРІРЅРѕР№ video-path РґР»СЏ WB Stream Рё Telemost |
| `seichannel` | payload РІ H264 SEI NAL units, ACK/retry | fallback РґР»СЏ WB Stream / Jitsi|
| `videochannel` | QR/tile РєР°РґСЂС‹ С‡РµСЂРµР· ffmpeg, ACK/retry | СЌРєСЃРїРµСЂРёРјРµРЅС‚Р°Р»СЊРЅС‹Р№ РІРёР·СѓР°Р»СЊРЅС‹Р№ С‚СЂР°РЅСЃРїРѕСЂС‚ |

Р РµРєРѕРјРµРЅРґСѓРµРјС‹Р№ СЃС‚Р°СЂС‚: `jitsi + datachannel`. РђР»СЊС‚РµСЂРЅР°С‚РёРІР°: `wbstream + vp8channel`.

## РЁРёС„СЂРѕРІР°РЅРёРµ Рё handshake

`internal/crypto` РёСЃРїРѕР»СЊР·СѓРµС‚ XChaCha20-Poly1305. РћР±С‰РёР№ РєР»СЋС‡ Р·Р°РґР°С‘С‚СЃСЏ РєР°Рє 64 hex-СЃРёРјРІРѕР»Р°:

```bash
openssl rand -hex 32
```

РџРѕРІРµСЂС… Р·Р°С€РёС„СЂРѕРІР°РЅРЅРѕРіРѕ `muxconn` Р·Р°РїСѓСЃРєР°РµС‚СЃСЏ `smux`. РџРµСЂРІС‹Р№ smux stream Р·Р°РЅСЏС‚ handshake Рё control protocol:

```text
CLIENT_HELLO -> SERVER_WELCOME
CONTROL_PING <-> CONTROL_PONG
```

Р•СЃР»Рё control pong РЅРµ РїСЂРёС…РѕРґРёС‚ РЅРµСЃРєРѕР»СЊРєРѕ СЂР°Р· РїРѕРґСЂСЏРґ, runtime РїРµСЂРµСЃРѕР±РёСЂР°РµС‚ smux-СЃРµСЃСЃРёСЋ РёР»Рё РѕС‚РґР°С‘С‚ СѓРїСЂР°РІР»РµРЅРёРµ failover supervisor.

## YAML

РњРёРЅРёРјР°Р»СЊРЅС‹Р№ СЃРµСЂРІРµСЂ:

```yaml
mode: srv
auth:
  provider: jitsi
room:
  # РСЃРїРѕР»СЊР·СѓР№С‚Рµ С‚РѕС‚ Jitsi-СЃРµСЂРІРµСЂ, РєРѕС‚РѕСЂС‹Р№ СЂР°Р±РѕС‚Р°РµС‚ РІ РІР°С€РµР№ СЃРµС‚Рё:
  # https://meet1.arbitr.ru/ROOM  РёР»Рё  https://meet.cryptopro.ru/ROOM
  id: "https://meet1.arbitr.ru/REPLACE_ME_WITH_ROOM_ID"
crypto:
  key: "REPLACE_ME_WITH_64_HEX_CHARS"
net:
  transport: datachannel
  dns: "8.8.8.8:53"
data: data
```

РњРёРЅРёРјР°Р»СЊРЅС‹Р№ РєР»РёРµРЅС‚:

```yaml
mode: cnc
auth:
  provider: jitsi
room:
  # РСЃРїРѕР»СЊР·СѓР№С‚Рµ С‚РѕС‚ Jitsi-СЃРµСЂРІРµСЂ, РєРѕС‚РѕСЂС‹Р№ СЂР°Р±РѕС‚Р°РµС‚ РІ РІР°С€РµР№ СЃРµС‚Рё:
  # https://meet1.arbitr.ru/ROOM  РёР»Рё  https://meet.cryptopro.ru/ROOM
  id: "https://meet1.arbitr.ru/REPLACE_ME_WITH_ROOM_ID"
crypto:
  key: "REPLACE_ME_WITH_64_HEX_CHARS"
net:
  transport: datachannel
  dns: "8.8.8.8:53"
socks:
  host: "127.0.0.1"
  port: 8808
data: data
```

РџРѕРґСЂРѕР±РЅРµРµ: [configuration.md](configuration.md), [settings.md](settings.md).

## Failover

`profiles[]` РїРѕР·РІРѕР»СЏРµС‚ Р·Р°РїСѓСЃРєР°С‚СЊ РЅРµСЃРєРѕР»СЊРєРѕ РєРѕРЅС„РёРіСѓСЂР°С†РёР№ РїРѕ РїРѕСЂСЏРґРєСѓ. РќР°РїСЂРёРјРµСЂ, СЃРЅР°С‡Р°Р»Р° `wbstream + vp8channel`, РїРѕС‚РѕРј `jitsi + datachannel`. Р’РµСЂС…РЅРµСѓСЂРѕРІРЅРµРІС‹Рµ РїРѕР»СЏ СЂР°Р±РѕС‚Р°СЋС‚ РєР°Рє defaults, РїСЂРѕС„РёР»СЊ РїРµСЂРµРѕРїСЂРµРґРµР»СЏРµС‚ С‚РѕР»СЊРєРѕ РЅСѓР¶РЅС‹Рµ С‡Р°СЃС‚Рё.

РђРєС‚РёРІРЅС‹Рµ smux streams РїСЂРё СЃРјРµРЅРµ РїСЂРѕС„РёР»СЏ РЅРµ РјРёРіСЂРёСЂСѓСЋС‚. РќРѕРІС‹Рµ РїРѕРґРєР»СЋС‡РµРЅРёСЏ СЃРјРѕРіСѓС‚ РїРѕРґРЅСЏС‚СЊСЃСЏ РЅР° СЃР»РµРґСѓСЋС‰РµРј РїСЂРѕС„РёР»Рµ.

## РЎС‚СЂСѓРєС‚СѓСЂР° СЂРµРїРѕР·РёС‚РѕСЂРёСЏ

| РџСѓС‚СЊ | Р§С‚Рѕ РІРЅСѓС‚СЂРё |
|---|---|
| `cmd/olcrtc` | CLI entrypoint |
| `cmd/olcrtc-cgo` | c-shared entrypoint |
| `pkg/olcrtc` | embeddable client/engine API |
| `pkg/olcrtc/tunnel` | embeddable server tunnel API |
| `mobile` | gomobile bindings РґР»СЏ Android |
| `internal/config` | YAML parsing, `crypto.key_file` |
| `internal/app/session` | defaults, validation, routing РІ `srv`/`cnc`/`gen` |
| `internal/auth` | provider-specific credential flows |
| `internal/engine` | SFU/signaling implementations |
| `internal/transport` | datachannel/vp8/sei/video transports |
| `internal/server` | server-side smux, handshake, TCP dial |
| `internal/client` | SOCKS5 listener, client-side smux |
| `internal/control` | liveness ping/pong |
| `internal/supervisor` | failover profiles |
| `script` | РёРЅС‚РµСЂР°РєС‚РёРІРЅС‹Рµ launchers Рё Docker entrypoint |
| `docs` | РґРѕРєСѓРјРµРЅС‚Р°С†РёСЏ Рё РїСЂРёРјРµСЂС‹ YAML |

## РЎР±РѕСЂРєР°

```bash
go install github.com/magefile/mage@latest

mage build
mage cross
mage test
mage lint
mage mobile
mage docker
mage podman
```

Go РІРµСЂСЃРёСЏ РІ СЃР±РѕСЂРѕС‡РЅС‹С… СЃРєСЂРёРїС‚Р°С…: `1.25`. Р”Р»СЏ `videochannel` РЅСѓР¶РµРЅ `ffmpeg`; РґР»СЏ `codec: tile` С‚СЂРµР±СѓРµС‚СЃСЏ СЂР°Р·СЂРµС€РµРЅРёРµ `1080x1080`.

## Public API

`pkg/olcrtc` РІРѕР·РІСЂР°С‰Р°РµС‚ `net.Conn`-РїРѕРґРѕР±РЅС‹Р№ РѕР±СЉРµРєС‚ РїРѕРІРµСЂС… auth/engine:

```go
sess, err := olcrtc.New(ctx, olcrtc.Config{
    Auth:   "jitsi",
    // РСЃРїРѕР»СЊР·СѓР№С‚Рµ meet1.arbitr.ru РёР»Рё meet.cryptopro.ru - С‚РѕС‚, С‡С‚Рѕ СЂР°Р±РѕС‚Р°РµС‚ РІ РІР°С€РµР№ СЃРµС‚Рё
    RoomID: "https://meet1.arbitr.ru/myroom",
})
if err != nil {
    return err
}
conn, err := sess.Dial(ctx)
```

`pkg/olcrtc/tunnel` РІСЃС‚СЂР°РёРІР°РµС‚ СЃРµСЂРІРµСЂРЅСѓСЋ СЃС‚РѕСЂРѕРЅСѓ Рё РґР°С‘С‚ hooks:

```go
srv := tunnel.New(tunnel.Config{
    Transport: "datachannel",
    Carrier:   "jitsi",
    // РСЃРїРѕР»СЊР·СѓР№С‚Рµ meet1.arbitr.ru РёР»Рё meet.cryptopro.ru - С‚РѕС‚, С‡С‚Рѕ СЂР°Р±РѕС‚Р°РµС‚ РІ РІР°С€РµР№ СЃРµС‚Рё
    RoomURL:   "https://meet1.arbitr.ru/myroom",
    KeyHex:    "<64-char hex>",
    DNSServer: "8.8.8.8:53",
})
err := srv.Run(ctx)
```

Р’ СЌС‚РѕРј API РїРѕР»Рµ `Carrier` СЃРѕС…СЂР°РЅРµРЅРѕ СЂР°РґРё СЃРѕРІРјРµСЃС‚РёРјРѕСЃС‚Рё СЃ СЃСѓС‰РµСЃС‚РІСѓСЋС‰РёРјРё РёРЅС‚РµРіСЂР°С†РёСЏРјРё; РїРѕ СЃРјС‹СЃР»Сѓ СЌС‚Рѕ РёРјСЏ `auth.provider`.

## Mobile / Android

`mobile/mobile.go` РїСЂРµРґРѕСЃС‚Р°РІР»СЏРµС‚ gomobile API:

- `SetProtector` РґР»СЏ Android VPN `protect(fd)`;
- `SetTransport`, `SetDNS`, `SetVP8Options`, `SetLivenessOptions`;
- `Start`, `StartWithTransport`, `Stop`;
- `Check`/ping helpers РґР»СЏ РїСЂРѕРІРµСЂРєРё РґРѕСЃС‚СѓРїРЅРѕСЃС‚Рё.

РџРѕ СѓРјРѕР»С‡Р°РЅРёСЋ mobile-РєР»РёРµРЅС‚ РёСЃРїРѕР»СЊР·СѓРµС‚ `vp8channel`; `datachannel` С‚РѕР¶Рµ РїРѕРґРґРµСЂР¶РёРІР°РµС‚СЃСЏ.

## РўРµСЃС‚С‹

```bash
go test -count=1 ./...
mage test
mage e2e
```

Real-provider E2E РІРєР»СЋС‡Р°СЋС‚СЃСЏ С‡РµСЂРµР· РїРµСЂРµРјРµРЅРЅС‹Рµ:

```bash
E2E_CARRIERS=wbstream E2E_TRANSPORTS= vp8channel mage e2e
```

## Р§Р°СЃС‚С‹Рµ РїСЂРѕР±Р»РµРјС‹

| РЎРёРјРїС‚РѕРј | Р§С‚Рѕ РїСЂРѕРІРµСЂРёС‚СЊ |
|---|---|
| `key required` РёР»Рё `invalid key` | РЅР° РѕР±РµРёС… СЃС‚РѕСЂРѕРЅР°С… РѕРґРёРЅР°РєРѕРІС‹Р№ 64-СЃРёРјРІРѕР»СЊРЅС‹Р№ hex key |
| SOCKS5 РЅРµ СЃР»СѓС€Р°РµС‚ | `mode: cnc`, `socks.host`, `socks.port`, Р»РѕРіРё РєР»РёРµРЅС‚Р° |
| Jitsi РЅРµ СЃРѕРµРґРёРЅСЏРµС‚СЃСЏ Р±РµР· РІС‚РѕСЂРѕРіРѕ СѓС‡Р°СЃС‚РЅРёРєР° | СЃРµСЂРІРµСЂ Рё РєР»РёРµРЅС‚ РґРѕР»Р¶РЅС‹ Р±С‹С‚СЊ РІ РѕРґРЅРѕР№ РєРѕРјРЅР°С‚Рµ |
| WB Stream + datachannel РЅРµ СЂР°Р±РѕС‚Р°РµС‚ | РІ guest flow РЅРµС‚ `canPublishData`; РёСЃРїРѕР»СЊР·СѓР№ `vp8channel`, `seichannel` РёР»Рё `videochannel` |
| `seichannel ack timeout` | РїСЂРѕРІР°Р№РґРµСЂ СЂРµР¶РµС‚/РЅРµ РјР°СЂС€СЂСѓС‚РёР·РёСЂСѓРµС‚ video path; СЃРјРµРЅРё transport/provider |
| `ffmpeg` not found | СѓСЃС‚Р°РЅРѕРІРё ffmpeg РёР»Рё Р·Р°РґР°Р№ `ffmpeg: /path/to/ffmpeg` |

## РЎСЃС‹Р»РєРё

- [Р‘С‹СЃС‚СЂС‹Р№ СЃС‚Р°СЂС‚](fast.md)
- [Р СѓС‡РЅР°СЏ СЃР±РѕСЂРєР°](manual.md)
- [РќР°СЃС‚СЂРѕР№РєР° YAML](configuration.md)
- [РњР°С‚СЂРёС†Р° СЃРѕРІРјРµСЃС‚РёРјРѕСЃС‚Рё](settings.md)
- [URI С„РѕСЂРјР°С‚](uri.md)
- [Р¤РѕСЂРјР°С‚ РїРѕРґРїРёСЃРєРё](sub.md)
