# homelab-gitops

Exported, cleaned YAML from the live Kubernetes cluster, intended as the starting
point for a GitOps repo (ArgoCD). Cluster-generated noise (resourceVersion, uid,
status blocks, last-applied-config annotations, creationTimestamp) has been stripped.

## Structure

- `media/` — gotify, igotify, bark, plex, jellyfin, *arr stack, vpn-stack, etc.
- `monitoring/` — Prometheus, Alertmanager, kube-state-metrics, node-exporter, blackbox-exporter
- `cert-manager/` — see README in that folder; recommend managing via Helm chart in ArgoCD rather than static YAML
- `tools/` — proxmox-mcp

## Before you commit this to Git

1. **Secrets.** Two things in here currently contain sensitive values that must NOT
   go into Git in plaintext:
   - `media/deployments.yaml` — the `vpn-stack` Deployment references a WireGuard
     private key via `secretKeyRef` (already redacted from the file). You need to
     create the `vpn-stack-secrets` Secret out-of-band before applying.
   - `tools/deployment.yaml` — `proxmox-mcp` references a `proxmox-mcp-config`
     Secret (API credentials) that is not exported here at all.
   - Consider using [sealed-secrets](https://github.com/bitnami-labs/sealed-secrets)
     or [SOPS](https://github.com/getsops/sops) so secrets *can* live in Git safely.

2. **Cleanup candidates.** `monitoring/deployments.yaml` and
   `monitoring/configmaps.yaml` both flag `alertmanager-ntfy` — it looks unused
   (the live `alertmanager-config` only routes to `gotify-alertmanager`, and
   `alertmanager-ntfy-config` points at a `ntfy.media.svc.cluster.local` service
   that doesn't exist in the cluster). Verify before deleting.

3. **Keel annotations.** Several Deployments still carry `keel.sh/*` annotations
   for automatic image updates. Per the migration plan, these should be removed
   and replaced with ArgoCD Image Updater annotations on an app-by-app basis once
   ArgoCD is managing that namespace — don't do this in bulk.

## Next steps

1. Create a Git repo and push this structure as the initial commit.
2. Install ArgoCD in a new `argocd` namespace (pure addition, no impact on existing apps).
3. Point ArgoCD Applications at this repo, starting with `tools` (lowest risk),
   then `cert-manager` (via Helm chart — see its README), then `monitoring`,
   then `media` last.
4. Once each namespace is confirmed syncing cleanly, swap Keel annotations for
   ArgoCD Image Updater on that namespace's apps.
