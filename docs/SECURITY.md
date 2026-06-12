# Security Model

## Threat model

The primary adversary is a **contestant** who controls arbitrary code running inside
a sandbox container and may try to (a) cheat the scoring, or (b) escape the sandbox
to reach platform infrastructure (Kafka, Redis, databases) and exfiltrate
credentials or other contestants' data. A secondary adversary is an external
attacker hitting the public API.

## Defense in depth: 7 layers on contestant containers

1. **seccomp** (`infra/docker/contestant/seccomp/contestant-profile.json`) — default
   `SCMP_ACT_ERRNO`; only the minimal syscall set for a network server is allowed.
   `fork`/`ptrace`/`mount` and ~200 others are blocked.
2. **AppArmor** (`infra/docker/contestant/apparmor/contestant-profile`) — denies
   writes outside `/tmp`, denies `/proc` and `/sys` writes, denies ptrace.
3. **Docker constraints** (`build-worker/sandbox.go`) — `CapDrop: ALL`,
   `ReadonlyRootfs`, 512 MB memory hard cap, `no-new-privileges`, `PidsLimit: 50`,
   pinned CPUs.
4. **Network isolation** — the `contestant-isolated` Docker network is `internal`
   (no route off-host); the Kubernetes `contestant-isolation` NetworkPolicy denies
   all egress and allows ingress only from `bot-fleet` on port 8080.
5. **Image scanning** — Trivy scans built images for HIGH/CRITICAL CVEs before run
   (CI `security.yml`).
6. **Resource monitoring** — build-worker's health monitor kills unhealthy/abusive
   containers; outbound traffic on non-8080 ports is a red flag.
7. **Kubernetes PodSecurity** — the `trade-eval` namespace enforces the `baseline`
   profile (no privileged containers, no host namespaces).

Even if one layer is bypassed, the network isolation (layer 4) means a compromised
container still cannot reach Kafka/Redis/Postgres.

## Scoring integrity

Correctness is judged by an **authoritative shadow order book** in the telemetry
ingester, replaying every contestant's orders in true send order (via the reorder
buffer + sequence numbers). A contestant cannot fake fills: the reference engine
decides what *should* have happened. Anomaly detection flags suspiciously-low
latency or perfect-correctness-at-impossible-speed for manual review.

## API security

- API-key auth on all mutating endpoints; 404 (not 403) on cross-contestant reads
  so existence isn't leaked.
- Per-contestant submission rate limit (10/hour) and per-IP leaderboard rate limit.
- Zip-slip guard on uploads; bounded extraction size.
- Constant-time comparison for the admin key.

## Infrastructure security

- IRSA (IAM Roles for Service Accounts) — no long-lived AWS keys in pods.
- Terraform remote state encrypted in S3 with DynamoDB locking.
- Secrets via AWS Secrets Manager / External Secrets Operator, never committed.
