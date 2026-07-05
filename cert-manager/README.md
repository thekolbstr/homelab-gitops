# cert-manager

This namespace runs the standard Jetstack cert-manager v1.20.2 controller, cainjector,
and webhook, installed via the official manifest/Helm chart, not hand-rolled YAML.

Rather than hand-writing (and having to keep in sync with upstream) the full
Deployment/Service/RBAC/CRD set, this is a strong candidate to manage via ArgoCD as a
Helm-based Application pointing directly at the upstream cert-manager Helm chart
(https://charts.jetstack.io) pinned to version v1.20.2, rather than committing static
YAML here. That keeps upgrades a one-line version bump instead of manual re-export.

If you'd rather vendor static manifests instead, the live resources are:
- Deployments: cert-manager, cert-manager-cainjector, cert-manager-webhook
- Services: cert-manager, cert-manager-cainjector, cert-manager-webhook
- Plus a large set of CRDs, ClusterRoles/Bindings, ValidatingWebhookConfiguration,
  MutatingWebhookConfiguration not yet exported in this pass.

Recommendation: use the Helm chart route in ArgoCD for this namespace.
