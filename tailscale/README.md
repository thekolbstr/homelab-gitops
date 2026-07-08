# Tailscale subnet router

Advertises `192.168.0.0/24` into your tailnet so devices connected via
Tailscale can reach anything on the home LAN, including Traefik-fronted
`*.home.arpa` services.

## One-time setup

1. Generate an auth key: Tailscale admin console -> Settings -> Keys ->
   Generate auth key. Recommended: **Reusable**, **Ephemeral: off**,
   **Tags**: none needed unless you use ACL tags, expiry as you like
   (90 days is the default; you can always regenerate).

2. Create the secret in-cluster (not committed to git):
   ```
   kubectl create namespace tailscale
   kubectl create secret generic tailscale-auth -n tailscale \
     --from-literal=TS_AUTHKEY=<paste-your-authkey-here>
   ```

3. Apply this app (or let ArgoCD do it once argocd-apps/tailscale.yaml is applied):
   ```
   kubectl apply -f argocd-apps/tailscale.yaml
   ```

4. In the Tailscale admin console, go to Machines, find `k3s-subnet-router`,
   and click "..." -> Edit route settings -> approve the `192.168.0.0/24`
   route. Routes are advertised automatically but require manual approval
   before they're usable.

5. Configure Split DNS for `home.arpa`: Tailscale admin console -> DNS ->
   Nameservers -> Add nameserver -> point it at your Ubuntu Pi-hole's LAN
   IP -> restrict to search domain `home.arpa` (toggle "Restrict to domain").
   This makes `*.home.arpa` resolve correctly for any device on your tailnet,
   without changing global DNS for the whole tailnet.

## Notes

- If you ever rotate the auth key, just re-create the secret and roll the
  deployment (`kubectl rollout restart deployment subnet-router -n tailscale`).
- State (identity/keys) persists in `tailscale-state-pvc` (Longhorn), so pod
  restarts don't require re-authenticating -- only re-creating the PVC would.
