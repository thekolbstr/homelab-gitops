# homelab-gitops

GitOps repo for the home Kubernetes cluster, managed by ArgoCD. Each
top-level directory is a Kustomize target, and most map 1:1 to both a
Kubernetes namespace and an ArgoCD Application defined in `argocd-apps/`.

## Structure

- `argocd-apps/` - ArgoCD Application manifests, one per app namespace
  (bark, gotify, homepage, igotify, jellyfin, ollama, open-webui, plex,
  radarr, seerr, sonarr, tautulli, tdarr). Each Application has
  `automated.selfHeal: true` and `automated.prune: false`. These
  Application objects themselves are applied directly via `kubectl apply`
  (not bootstrapped via an app-of-apps pattern), so if you ever rebuild
  ArgoCD from scratch, re-apply everything in this directory.
- `bark/`, `gotify/`, `igotify/`, `tautulli/`, `jellyfin/`, `radarr/`,
  `sonarr/`, `plex/`, `homepage/`, `ollama/`, `open-webui/`, `seerr/`,
  `tdarr/` - one namespace per app, each containing its Deployment,
  Service, Ingress, and PVC(s) in `resources.yaml`.
- `media/` - only vpn-stack now (gluetun + dispatcharr + sabnzbd +
  qbittorrent + prowlarr + byparr sharing one pod, since they route
  through gluetun's network namespace for VPN tunneling - this can't be
  split into separate pods/namespaces without breaking that routing),
  plus the shared `appdata-pvc` / `media-data-pvc` claims vpn-stack still
  needs and `dispatcharr-db-pvc`.
- `monitoring/` - Prometheus, Alertmanager, kube-state-metrics,
  node-exporter, blackbox-exporter.
- `cert-manager/` - managed via Helm chart in ArgoCD rather than static
  YAML; see that directory for details.
- `kube-system/` - cluster add-ons (coredns, local-path-provisioner,
  metrics-server, nvidia-device-plugin, traefik, headlamp).
- `tools/` - proxmox-mcp.

## Shared storage

`appdata-pv` and `media-data-pv` are NFS exports from a Synology NAS at
`192.168.0.183` (`/volume1/Storage/appdata` and `/volume1/Storage/data`).
A single PersistentVolumeClaim can only be mounted by pods in its own
namespace, and a PersistentVolume can only bind to one PVC at a time -
so apps in different namespaces that need this shared data each get
their own PV/PVC pointing at the same NFS path (e.g. `media-data-pv-plex`,
`media-data-pv-radarr`, `appdata-pv-ollama`, etc.), rather than one PVC
being referenced across namespaces. This gives every app real-time
shared access to the same underlying files with no duplication.

Apps with their own dedicated per-app volume (jellyfin, radarr, sonarr,
plex, tautulli, bark, gotify, igotify) use Longhorn with
`accessModes: ReadWriteOnce` on the `longhorn` storageclass. These used
to be `ReadWriteMany` on `longhorn`/`longhorn-single`, which Longhorn
serves via an NFS share-manager pod rather than a native block device -
running SQLite (WAL mode, file locking) over that NFS layer caused
database corruption and "database is busy" write-lock timeouts,
most visibly breaking Plex Live TV. None of these volumes are actually
shared across pods, so RWO block storage is both correct and faster.

Because RollingUpdate can't work with ReadWriteOnce volumes (the new
pod can't attach the volume while the old pod still holds it, causing a
FailedAttachVolume deadlock), every Deployment using one of these RWO
volumes has `strategy.type: Recreate`.

## Image updates

`argocd-image-updater` auto-updates most images by tracking the
`:latest` tag (see the `argocd-image-updater.argoproj.io/*` annotations
on the `media` Application). Plex is the exception - it tracks versioned
release tags via `allow-tags` + `update-strategy: newest-build` instead
of `:latest`, so it still auto-updates but only to a real numbered
release rather than whatever `latest` happens to resolve to on a given
day.

## Secrets

Not committed here, must be created out-of-band:

- `vpn-stack-secrets` (namespace `media`) - WireGuard private key,
  referenced by the vpn-stack Deployment's gluetun container.
- `proxmox-mcp-config` (namespace `tools`) - proxmox-mcp API credentials.

Consider [sealed-secrets](https://github.com/bitnami-labs/sealed-secrets)
or [SOPS](https://github.com/getsops/sops) if you want these to live in
Git safely instead of being applied manually.

## Sync policy

Every ArgoCD Application has `selfHeal: true` and `prune: false`: manual
`kubectl` changes to anything ArgoCD manages will be reverted
automatically on the next sync unless committed to Git first. `prune`
staying `false` means deleting a resource from Git won't delete it live -
that has to be done manually. When doing hands-on cluster debugging,
temporarily disable automated sync on the relevant Application first
(`kubectl patch application <name> -n argocd --type merge -p
'{"spec":{"syncPolicy":{"automated":{"selfHeal":false}}}}'`), then
re-enable once done.

## Cleanup candidates

`monitoring/deployments.yaml` and `monitoring/configmaps.yaml` both
reference `alertmanager-ntfy` - it looks unused (the live
`alertmanager-config` only routes to `gotify-alertmanager`, and
`alertmanager-ntfy-config` points at a `ntfy.media.svc.cluster.local`
service that doesn't exist in the cluster). Verify before deleting.