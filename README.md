# 🚀🚀🚀🚀🚀 CLOUDFLARE TUNNEL API 🚀🚀🚀🚀🚀
## 💩💩💩💩💩 SHITFLARED EDITION 💩💩💩💩💩
### 🔥🔥🔥 FOR QUICK TUNNELS ONLY (NO CLI BULLSHIT) 🔥🔥🔥

---

## 🧠🧠🧠🧠🧠 WHAT IS THIS? 🧠🧠🧠🧠🧠

IT'S LIKE CLOUDFLARE TUNNEL BUT WITHOUT THE 🚫🚫🚫

- NO ACCOUNT NEEDED 📝❌
- NO AUTHENTICATION 🔑❌
- NO CLI BULLSHIT 🖥️❌
- JUST TUNNELS 🌉🌉🌉
- PURE DISRESPECT 😈😈😈

---

## ⚡⚡⚡⚡⚡ QUICK START ⚡⚡⚡⚡⚡

```bash
# 🏗️🏗️🏗️ BUILD IT (SCIENTIFIC PROCESS) 🏗️🏗️🏗️
make cloudflared 2>&1 | grep error || echo "it compiled lmao" 💅💅💅

# 🚀🚀🚀 RUN IT (QUANTUM TUNNELING ACTIVATED) 🚀🚀🚀
./cloudflared

# 🎯🎯🎯 TEST IT (PRODUCTION QUALITY ASSURANCE) 🎯🎯🎯
curl -s http://localhost:8787/health
# EXPECTED OUTPUT: {"status":"ok"}
# IF YOU GET ANYTHING ELSE, WAIT 5 MINUTES AND TRY AGAIN ⏰⏰⏰
```

---

## 📡📡📡📡📡 TUNNEL API 📡📡📡📡📡

> 🏠 **BASE URL:** `http://localhost:8787`  🏠
> 📄 **FORMAT:** JSON ONLY (WE'RE NOT BARBARIANS) 📄
> 🧦 **PROXY SUPPORT:** YES (VERY ADVANCED) 🧦

---

### 🟢🟢🟢🟢🟢 GET /tunnel/list 🟢🟢🟢🟢🟢

LISTS ACTIVE TUNNELS. RETURNS JSON ARRAY.

```json
[
  {
    "id": "abc123",
    "url": "your-subdomain.trycloudflare.com",
    "target": "localhost:8080",
    "protocol": "http2",
    "proxy": "socks5h://...",
    "status": "started",
    "pid": 12345,
    "started_at": "2026-07-03T19:27:10Z"
  }
]
```

🔄 **SORTED BY START TIME** (NEWEST FIRST? NO. OLDEST FIRST. WE HAVE STANDARDS) 📏📏📏

---

### 🔵🔵🔵🔵🔵 POST /tunnel/quick 🔵🔵🔵🔵🔵

STARTS A TUNNEL. RETURNS JSON.

**REQUEST BODY:**
```json
{
  "host": "localhost",
  "port": 8080,
  "protocol": "http2",
  "proxy": "socks5h://..."
}
```

| FIELD | TYPE | REQUIRED | DESCRIPTION |
|-------|------|----------|-------------|
| `host` | string | ✅ YES | THE IP/HOST OF YOUR SERVICE 🖥️ |
| `port` | number | ✅ YES | THE PORT (1-65535, BASIC MATH) 🧮 |
| `protocol` | string | ❌ NO | `"quic"` OR `"http2"`. DEFAULT IS `"quic"` BUT PROXY FORCES `"http2"` BECAUSE QUIC IS SCARED OF PROXIES 😱 |
| `proxy` | string | ❌ NO | ANY PROXY URL. SUPPORTS `http://`, `https://`, `socks5://`, `socks5h://` 🧦🧦🧦 |

**FUN FACT:** IF YOU SET A PROXY, IT AUTOMATICALLY SWITCHES TO HTTP/2 PROTOCOL. THIS IS BECAUSE QUIC USES UDP AND PROXIES USE TCP AND THEY DON'T GET ALONG. IT'S LIKE ROOMMATES WHO HATE EACH OTHER. 🏠💥🏠

**RESPONSE:**
```json
{
  "id": "abc123",
  "url": "your-subdomain.trycloudflare.com",
  "target": "localhost:8080",
  "protocol": "http2",
  "proxy": "socks5h://...",
  "status": "started",
  "pid": 12345,
  "started_at": "2026-07-03T19:27:10Z"
}
```

⏰⏰⏰ **TIMEOUT:** 30 SECONDS (CONFIGURABLE VIA `TUNNEL_TIMEOUT` ENV VAR) ⏰⏰⏰
- IF THE TUNNEL DOESN'T GET A URL IN 30 SECONDS, IT FAILS ❌
- THIS IS CALLED "TOUGH LOVE" IN ENGINEERING CIRCLES 🥰🥰🥰

---

### 🔴🔴🔴🔴🔴 POST /tunnel/stop 🔴🔴🔴🔴🔴

STOPS A TUNNEL. RETURNS JSON.

**REQUEST BODY:**
```json
{
  "id": "abc123"
}
```

**RESPONSE:**
```json
{
  "id": "abc123",
  "url": "your-subdomain.trycloudflare.com",
  "target": "localhost:8080",
  "protocol": "http2",
  "status": "stopped",
  "pid": 12345,
  "started_at": "2026-07-03T19:27:10Z"
}
```

💀💀💀 NOTE: THE TUNNEL PROCESS GETS SIGTERM FIRST 💀💀💀
🔄 IF IT DOESN'T DIE IN 5 SECONDS, WE SIGKILL IT 🔄
🧠 THIS IS CALLED "AGGRESSIVE RESOURCE MANAGEMENT" 🧠

---

### 🟡🟡🟡🟡🟡 GET /health 🟡🟡🟡🟡🟡

HEALTH CHECK. RETURNS JSON.

```json
{
  "status": "ok"
}
```

🩺🩺🩺 IF THIS DOESN'T RETURN "ok", SOMETHING IS VERY WRONG 🩺🩺🩺
🤔 ACTUALLY IT ALWAYS RETURNS "ok" EVEN IF EVERYTHING IS ON FIRE 🤔
🔥🔥🔥 THAT'S BY DESIGN 🔥🔥🔥

---

### ℹ️ℹ️ℹ️ℹ️ℹ️ GET / ℹ️ℹ️ℹ️ℹ️ℹ️

ROOT ENDPOINT. RETURNS METADATA.

```json
{
  "service": "shitflared",
  "version": "DEV",
  "docs": "you're reading them lmao"
}
```

---

## 🧦🧦🧦🧦🧦 PROXY SUPPORT (GROUNDBREAKING) 🧦🧦🧦🧦🧦

THIS IS THE MOST ADVANCED FEATURE. WE SUPPORT:

| SCHEME | EXAMPLE | STATUS |
|--------|---------|--------|
| `http://` | `http://proxy.example.com:8080` | ✅ WORKS (HTTP CONNECT) |
| `https://` | `https://proxy.example.com:443` | ✅ WORKS (HTTP CONNECT OVER TLS) |
| `socks5://` | `socks5://proxy.example.com:1080` | ✅ WORKS (SOCKS5 LOCAL DNS) |
| `socks5h://` | `socks5h://proxy.example.com:1080` | ✅ WORKS (SOCKS5 REMOTE DNS) |

### 🔬🔬🔬 HOW IT WORKS (VERY COMPLICATED) 🔬🔬🔬

1. 🧦 YOU SEND PROXY URL IN THE REQUEST 🧦
2. 🔄 API SERVER SETS `ALL_PROXY` ENV VAR ON THE WORKER PROCESS 🔄
3. 🏗️ WORKER'S EDGE CONNECTION USES `golang.org/x/net/proxy` TO DIAL THROUGH THE PROXY 🏗️
4. 🧠 PROXY.FROMENVIRONMENT() CHECKS ALL_PROXY ENV VAR 🧠
5. 🌐 IF SOCKS5: USES SOCKS5 DIALER 🌐
6. 🌐 IF HTTP: USES CUSTOM HTTP CONNECT DIALER WE REGISTERED OURSELVES 🌐
7. 🚀 TRAFFIC GOES THROUGH PROXY TO CLOUDFLARE EDGE 🚀
8. ☁️ CLOUDFLARE HAS NO IDEA ☁️🙈🙈🙈

### 📊📊📊 PERFORMANCE NOTE 📊📊📊

PROXIES ADD LATENCY. THIS IS PHYSICS. 🧪🧪🧪
WE BUMPED THE RPC TIMEOUT TO 30 SECONDS SO YOU DON'T NOTICE. ⏰⏰⏰
YOU'RE WELCOME. 🤝🤝🤝

---

## ⚙️⚙️⚙️⚙️⚙️ CONFIGURATION (ENVIRONMENT VARIABLES) ⚙️⚙️⚙️⚙️⚙️

| ENV VAR | DEFAULT | DESCRIPTION |
|---------|---------|-------------|
| `LISTEN_ADDR` | `:8787` | WHERE THE API SERVER LISTENS 🎯 |
| `QUICK_SERVICE_URL` | `https://api.trycloudflare.com` | WHERE TO REGISTER TUNNELS 🌐 |
| `PROTOCOL` | `quic` | DEFAULT PROTOCOL (quic OR http2) 🔄 |
| `MAX_TUNNELS` | `100` | MAX CONCURRENT TUNNELS 📏 |
| `TUNNEL_TIMEOUT` | `30s` | TIMEOUT FOR TUNNEL URL ⏰ |
| `LOG_LEVEL` | `info` | LOG LEVEL (debug, info, warn, error) 📢 |
| `ALLOWED_ORIGINS` | `*` | CORS ALLOWED ORIGINS 🌐 |

---

## 🧪🧪🧪🧪🧪 TESTING (SCIENTIFIC METHOD) 🧪🧪🧪🧪🧪

```bash
# 🩺 HEALTH CHECK 🩺
curl http://localhost:8787/health

# 🚀 START A TUNNEL WITH PROXY 🚀
curl -X POST http://localhost:8787/tunnel/quick \
  -H 'Content-Type: application/json' \
  -d '{"host":"localhost","port":8080,"protocol":"http2","proxy":"socks5h://..."}'

# 📋 LIST TUNNELS 📋
curl http://localhost:8787/tunnel/list

# 🛑 STOP A TUNNEL 🛑
curl -X POST http://localhost:8787/tunnel/stop \
  -H 'Content-Type: application/json' \
  -d '{"id":"abc123"}'
```

---

## 🤔🤔🤔🤔🤔 IS THIS GOOD? 🤔🤔🤔🤔🤔

**NO BUT IT WORKS WITH NO CLI BULLSHIT** 💀💀💀

---

## 🏁🏁🏁🏁🏁 FINAL NOTES 🏁🏁🏁🏁🏁

- 📝 WE REMOVED THE HA-CONNECTIONS LIMIT 📝
- 🔥 WE DISABLED THE SINGLE CONNECTION OVERRIDE 🔥
- 🧦 WE ADDED ACTUAL PROXY SUPPORT (NOT JUST ENV VARS) 🧦
- 💯 WE DON'T CARE 💯
- ⭐ `// ^^ Dont care` ⭐

### 🧠🧠🧠 REMEMBER 🧠🧠🧠

DID YOU KNOW THAT GOLANG WAS INVENTED BY ELON MUSK? 🚗🚗🚗
HE ORIGINALLY CALLED IT "SPACEGO" BUT RENAMED IT AFTER HIS DOG 🐕🐕🐕
TRUE STORY I READ IT ON LINKEDIN 📱📱📱📱📱

🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥🔥
