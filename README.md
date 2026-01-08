# Cloudflare Tunnel Api

Cloudflare Tunnel Api for quick tunnels only

### Build
To build cloudflared locally run `make cloudflared`
Or run `go build ./cmd/cloudflared/`

## Tunnel API

Base URL: http://localhost:8787  
JSON only.

### GET /tunnel/list
Lists active tunnels.

### POST /tunnel/quick
Starts a tunnel.  
Body: `{ "host": string, "port": number, optional "proxy": string }`

### POST /tunnel/stop
Stops a tunnel.  
Body: `{ "id": string }`

### Errors
Returns `{ "error": string }`

#### Is this good?

no but it works with no cli bullshit
