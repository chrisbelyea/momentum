# Security and deployment policy

Production deployments must set `MOMENTUM_ENCRYPTION_KEY`; the server exits if it is absent. `MOMENTUM_DEV_MODE=1` enables only the local integration/development fallback and must not be used for exposed deployments.

When `TLS_CERT` and `TLS_KEY` are omitted, the server creates a self-signed
development certificate for localhost/loopback. This is not a production
certificate. The v0.2.2 listener binds all interfaces for the configured port,
so local users should keep it behind a host firewall; network deployments must
provide a trusted certificate and deliberately restrict exposure. Loopback-by-
default binding is tracked in [#143](https://github.com/chrisbelyea/momentum/issues/143).

External CalDAV configuration/validation accepts HTTPS on port 443 only, rejects
embedded credentials and private/loopback/link-local destinations, does not
follow redirects, and verifies certificates by default. `skip_tls_verify` is
limited to explicit loopback development targets. Runtime external
import/push synchronization is tracked in [#68](https://github.com/chrisbelyea/momentum/issues/68)
and [#144](https://github.com/chrisbelyea/momentum/issues/144).
