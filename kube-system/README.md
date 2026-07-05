# kube-system

Tracked node-level config that lives in the `kube-system` namespace.

## nvidia-device-plugin-config

ConfigMap consumed by the existing `nvidia-device-plugin-daemonset` DaemonSet
(installed out-of-band, not managed by this repo) to enable GPU time-slicing.
Configures `nvidia.com/gpu` with `replicas: 2` so multiple pods (ollama, tdarr)
can share the single physical GPU concurrently.

The DaemonSet itself was patched manually to add:
```
args: ["--config-file=/etc/nvidia/time-slicing.yaml"]
```
plus a volume mount of this ConfigMap at `/etc/nvidia`. That DaemonSet spec
is not yet fully captured in git — only the ConfigMap it depends on is
tracked here. If the DaemonSet is ever reinstalled/reset, the args/volume
mount will need to be reapplied manually or the full DaemonSet spec should
be added to this repo.
