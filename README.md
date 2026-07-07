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
- `argocd-notifications/` - `argocd-notifications-cm` (Gotify + Bark push
  notifications on ArgoCD sync succeeded/failed, subscribed globally to
  every Application) plus its SealedSecret. Backed by its own
  `argocd-notifications` Application.
- `sealed-secrets-controller/` - the sealed-secrets controller itself
  (CRD, RBAC, Deployment), installed via the upstream `controller.yaml`
  release manifest. Backed by its own `sealed-secrets-controller`
  Application, destination namespace `kube-system`.

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

Real secrets are encrypted with sealed-secrets and committed to Git as
`SealedSecret` resources - safe to keep in a repo even if it were public,
since only the in-cluster controller's private key can decrypt them:

- `media/sealedsecret.yaml` -> `vpn-stack-secrets` (WireGuard private
  key, used by the vpn-stack Deployment's gluetun container).
- `tools/sealedsecret.yaml` -> `proxmox-mcp-config` (proxmox-mcp API
  credentials).
- `argocd-notifications/sealedsecret.yaml` -> `argocd-notifications-secret`
  (Gotify app token, Bark device key).

**Disaster recovery - read this before wiping the cluster.** SealedSecrets
can only be decrypted by the exact controller keypair that sealed them.
If `sealed-secrets-controller` is ever reinstalled from scratch without
restoring its original key, every SealedSecret in this repo becomes
permanently unreadable. The controller's private key is backed up
outside Git (Kolby has it in a password manager / secure offline location
as of 2026-07-06) - restore that key into `kube-system` before the
controller starts on a rebuilt cluster:

    kubectl apply -f sealed-secrets-master-key-BACKUP.yaml
    kubectl rollout restart deployment sealed-secrets-controller -n kube-system

**Re-sealing / rotating a secret:** fetch the controller's public cert
with `kubeseal --fetch-cert --controller-namespace kube-system
--controller-name sealed-secrets-controller > cert.pem`, then pipe a
plain Secret manifest through `kubeseal --cert cert.pem --format yaml`
to produce a new SealedSecret and commit it.

**Adopting a pre-existing plain Secret:** if a plain Secret with the same
name/namespace already exists before you apply its SealedSecret, the
controller refuses to touch it (error: "failed update: Resource ...
already exists and is not managed by SealedSecret") and ArgoCD shows the
owning Application as Degraded. Delete the plain Secret first (data is
identical, so there's no disruption) and the SealedSecret controller
recreates it under its own ownership within a few seconds:

    kubectl delete secret <name> -n <namespace>

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