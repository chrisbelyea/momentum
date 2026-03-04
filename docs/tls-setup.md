# TLS Setup Guide

Momentum enforces TLS-only connections. The server will not start unless `TLS_CERT` and
`TLS_KEY` environment variables are set to valid certificate and private key files.
Plain HTTP clients are rejected; if you configure the optional HTTP redirect port, plain
HTTP requests are permanently redirected to HTTPS instead.

## Environment Variables

| Variable            | Required | Default | Description                                      |
|---------------------|----------|---------|--------------------------------------------------|
| `TLS_CERT`          | Yes      | —       | Path to the TLS certificate file (PEM format)    |
| `TLS_KEY`           | Yes      | —       | Path to the TLS private key file (PEM format)    |
| `PORT`              | No       | `8443`  | HTTPS listen port                                |
| `HTTP_REDIRECT_PORT`| No       | —       | If set, an HTTP server on this port redirects all requests to HTTPS |

> **Minimum TLS version**: TLS 1.3. Cipher suite selection is managed by Go's
> `crypto/tls` package, which only enables strong suites by default.

---

## Development Setup

For local development, generate a self-signed certificate using one of the methods below.

### Option A: mkcert (recommended)

[mkcert](https://github.com/FiloSottile/mkcert) creates locally-trusted certificates
with zero configuration and no browser warnings.

```bash
# Install mkcert (macOS example; see project README for other platforms)
brew install mkcert
mkcert -install          # installs root CA into system/browser trust stores

# Generate cert for localhost
mkcert localhost 127.0.0.1 ::1
# Creates: localhost+2.pem  localhost+2-key.pem
```

Start the server:

```bash
export TLS_CERT=localhost+2.pem
export TLS_KEY=localhost+2-key.pem
export PORT=8443
./bin/momentum-server
```

Then open `https://localhost:8443` in your browser.

### Option B: openssl self-signed certificate

Use this if you cannot install mkcert. Note that browsers will show a security warning
for self-signed certificates unless you add the cert to your trust store manually.

```bash
openssl req -x509 -newkey rsa:4096 -sha256 -days 365 -nodes \
  -keyout server.key -out server.crt \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
```

Start the server:

```bash
export TLS_CERT=server.crt
export TLS_KEY=server.key
./bin/momentum-server
```

---

## Production Setup

In production, use a certificate issued by a trusted Certificate Authority (CA).

### Let's Encrypt with Certbot

[Certbot](https://certbot.eff.org/) automates certificate issuance and renewal from
[Let's Encrypt](https://letsencrypt.org/) for publicly reachable domains.

```bash
# Install certbot (Debian/Ubuntu example)
sudo apt install certbot

# Obtain a certificate (standalone mode — no web server required during initial issuance)
sudo certbot certonly --standalone -d your.domain.com

# Certificates are written to:
#   /etc/letsencrypt/live/your.domain.com/fullchain.pem
#   /etc/letsencrypt/live/your.domain.com/privkey.pem
```

Start the server:

```bash
export TLS_CERT=/etc/letsencrypt/live/your.domain.com/fullchain.pem
export TLS_KEY=/etc/letsencrypt/live/your.domain.com/privkey.pem
export PORT=443
export HTTP_REDIRECT_PORT=80   # optional: redirects http:// to https://
./bin/momentum-server
```

#### Automatic renewal

Certbot installs a systemd timer (or cron job) that renews certificates automatically.
Because Momentum holds the key files open until restart, configure a renewal hook to
restart the server after renewal:

```bash
# /etc/letsencrypt/renewal-hooks/deploy/restart-momentum.sh
#!/bin/bash
systemctl restart momentum
```

```bash
chmod +x /etc/letsencrypt/renewal-hooks/deploy/restart-momentum.sh
```

### Other CAs / manually managed certificates

If you use a commercial CA or an internal PKI, obtain the certificate and key in PEM
format and set `TLS_CERT`/`TLS_KEY` to their paths. The requirements are:

- **Certificate**: PEM-encoded X.509 certificate (may include intermediate chain).
- **Private key**: PEM-encoded private key (RSA 2048+, ECDSA P-256/P-384, or Ed25519).
- **Validity**: Ensure the certificate is valid and not expired; renew before expiry.

---

## systemd Service Example

```ini
# /etc/systemd/system/momentum.service
[Unit]
Description=Momentum Task Server
After=network.target

[Service]
Type=simple
User=momentum
EnvironmentFile=/etc/momentum/env
ExecStart=/usr/local/bin/momentum-server
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

`/etc/momentum/env`:

```bash
TLS_CERT=/etc/letsencrypt/live/your.domain.com/fullchain.pem
TLS_KEY=/etc/letsencrypt/live/your.domain.com/privkey.pem
PORT=443
HTTP_REDIRECT_PORT=80
DB_PATH=/var/lib/momentum/momentum.db
MOMENTUM_ENCRYPTION_KEY=<your-strong-random-key>
```

> **Important**: Restrict permissions on the env file so only the `momentum` user can
> read it: `chmod 600 /etc/momentum/env`

---

## Security Checklist

- [ ] `TLS_CERT` and `TLS_KEY` point to valid, non-expired certificate files
- [ ] Private key file is readable only by the server process user (`chmod 600`)
- [ ] Certificate is issued by a trusted CA (or mkcert root CA for dev)
- [ ] `MOMENTUM_ENCRYPTION_KEY` is set to a strong random value in production
- [ ] HTTP redirect port is enabled, or HTTP port is firewalled/closed to prevent unnecessary connection attempts
- [ ] Certificate renewal is automated (certbot timer, cron, or equivalent)
- [ ] Firewall rules allow only the HTTPS port (and optionally HTTP redirect port) inbound

## References

- [mkcert](https://github.com/FiloSottile/mkcert) — locally trusted development certificates
- [Let's Encrypt](https://letsencrypt.org/) — free, automated, open CA
- [Certbot](https://certbot.eff.org/) — Let's Encrypt certificate automation
- [RFC 8446: TLS 1.3](https://datatracker.ietf.org/doc/html/rfc8446)
- [docs/security-implementation.md](security-implementation.md) — full security overview
