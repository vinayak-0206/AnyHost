# AnyHost Helm Chart

A Helm chart for deploying AnyHost - the self-hosted tunnel service for teams.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+
- PV provisioner support (if persistence is enabled)

## Installation

```bash
# Add the AnyHost Helm repository (coming soon)
helm repo add anyhost https://charts.anyhost.dev

# Install the chart
helm install anyhost anyhost/anyhost \
  --set config.domain=tunnel.example.com
```

### From Local Chart

```bash
cd deploy/helm
helm install anyhost ./anyhost \
  --set config.domain=tunnel.example.com
```

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Image repository | `anyhost/gotunnel` |
| `image.tag` | Image tag | `appVersion` |
| `config.domain` | Base domain for tunnels | `tunnel.example.com` |
| `config.logLevel` | Log level | `info` |
| `config.authMode` | Authentication mode | `token` |
| `secrets.authTokens` | Auth tokens (token1:user1,token2:user2) | `""` |
| `secrets.existingSecret` | Use existing secret | `""` |
| `persistence.enabled` | Enable persistence | `true` |
| `persistence.size` | PVC size | `1Gi` |
| `ingress.enabled` | Enable ingress | `false` |
| `ingress.wildcard.enabled` | Enable wildcard ingress | `false` |

See [values.yaml](values.yaml) for all configuration options.

## Examples

### Basic Installation

```bash
helm install anyhost ./anyhost \
  --set config.domain=tunnel.example.com \
  --set secrets.authTokens="mytoken:myuser"
```

### With Ingress and TLS

```bash
helm install anyhost ./anyhost \
  --set config.domain=tunnel.example.com \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=tunnel.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix \
  --set ingress.tls[0].secretName=tunnel-tls \
  --set ingress.tls[0].hosts[0]=tunnel.example.com \
  --set ingress.wildcard.enabled=true \
  --set ingress.wildcard.host="*.tunnel.example.com"
```

### With External Secret

```bash
# Create secret first
kubectl create secret generic anyhost-tokens \
  --from-literal=tokens="token1:user1
token2:user2"

# Install with existing secret
helm install anyhost ./anyhost \
  --set config.domain=tunnel.example.com \
  --set secrets.existingSecret=anyhost-tokens \
  --set secrets.existingSecretKey=tokens
```

## Persistence

The chart uses a PersistentVolumeClaim to store the SQLite database. If you disable persistence, data will be lost on pod restart.

```yaml
persistence:
  enabled: true
  storageClass: "standard"
  size: 5Gi
```

## Wildcard DNS

For subdomain routing to work, you need wildcard DNS configured:

```
*.tunnel.example.com -> Your Kubernetes Ingress IP
```

## Upgrading

```bash
helm upgrade anyhost anyhost/anyhost --reuse-values
```

## Uninstalling

```bash
helm uninstall anyhost
```

**Note:** This will not delete the PVC. To delete data:

```bash
kubectl delete pvc anyhost-data
```
