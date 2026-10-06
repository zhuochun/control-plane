# Local hostname

With `aicp serve` running, open <http://aicp.localhost:7331> in your browser.
aicp accepts this exact hostname alongside `localhost`, `127.0.0.1`, and `::1`;
it continues to listen only on `127.0.0.1:7331`.

`.localhost` names are reserved for loopback by
[RFC 6761](https://www.rfc-editor.org/rfc/rfc6761.html#section-6.3).
Browsers commonly resolve them automatically, but OS and CLI resolver support
can differ. If needed on Windows, add this exact entry to
`C:\Windows\System32\drivers\etc\hosts` using an administrator editor:

```text
127.0.0.1 aicp.localhost
```

## Open without a port

Use [Caddy](https://caddyserver.com/docs/install) as a local reverse proxy.
Keep `aicp serve` running, install Caddy, then run from the repository root
in another terminal:

```powershell
caddy validate --config examples/Caddyfile --adapter caddyfile
caddy run --config examples/Caddyfile --adapter caddyfile
```

The supplied [Caddyfile](../examples/Caddyfile) contains:

```caddyfile
http://aicp.localhost {
    bind 127.0.0.1 ::1
    reverse_proxy 127.0.0.1:7331
}
```

Open <http://aicp.localhost>. Caddy listens on loopback port 80 and forwards
requests to aicp on port 7331. The explicit `http://` keeps this setup on HTTP.
The direct URL <http://aicp.localhost:7331> remains available. Port 80 must be
free; if Caddy reports it is occupied, inspect the existing listener rather
than stopping an unrelated service.

Caddy [preserves the Host header for this HTTP upstream](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy#headers).
Keep the original Host and Origin headers so aicp's same-origin checks also
protect portal mutations through the proxy. Other hostnames and foreign
origins remain rejected.

Stop Caddy with Ctrl+C. This command runs in the foreground; automatic startup
requires a separate service or login-task setup. CLI and MCP can continue
using their default `http://127.0.0.1:7331` address. On another device,
`aicp.localhost` refers to that device, not the machine running aicp.
