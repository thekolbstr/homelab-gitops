# ArgoCD Notifications

`configmap.yaml` here mirrors the live `argocd-notifications-cm` in the `argocd`
namespace. It wires ArgoCD sync-succeeded/sync-failed events to Gotify and Bark
push notifications for every Application (global subscription).

Values referenced as `$gotify-token` and `$bark-device-key` are pulled from a
Secret named `argocd-notifications-secret` in the `argocd` namespace, which is
NOT stored in git (same pattern as `vpn-stack-secrets`). To recreate it on a
fresh cluster:

```
kubectl create secret generic argocd-notifications-secret -n argocd \
  --from-literal=gotify-token=<gotify application token> \
  --from-literal=bark-device-key=<bark device key from the iOS app>
```

The Gotify token is created via POST /application on the Gotify instance
(basic auth as an admin user). The Bark device key comes from the Bark iOS
app's home screen.