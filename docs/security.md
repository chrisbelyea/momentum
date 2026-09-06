# Security and deployment policy

Production deployments must set `MOMENTUM_ENCRYPTION_KEY`; the server exits if it is absent. `MOMENTUM_DEV_MODE=1` enables only the local integration/development fallback and must not be used for exposed deployments.

External CalDAV validation accepts HTTPS on port 443 only, rejects embedded credentials and private/loopback/link-local destinations, does not follow redirects, and verifies certificates by default. `skip_tls_verify` is limited to explicit loopback development targets.
