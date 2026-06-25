# Snooker no Kubernetes

Manifests para rodar o Snooker Multiplayer em um homelab com Debian, K3s e Headlamp.

## Extras declarados

| Codigo | Extra | Pontos | Onde demonstrar |
| --- | --- | ---: | --- |
| 5.2 | StatefulSet | 10 | `k8s/storage/postgres-statefulsets.yaml` e `k8s/storage/minio-statefulset.yaml` |
| 4.5 | NetworkPolicy | 8 | `k8s/networkpolicies.yaml` |
| 4.3 | Ingress Controller | 7 | `k8s/ingress.yaml` |
| 4.2 | Jobs | 5 | `k8s/jobs/*-job.yaml` |
|  | Total | 30 |  |

## Imagens usadas

```txt
maiconda/snooker-auth:v1.0.0
maiconda/snooker-profile:v1.0.0
maiconda/snooker-game:v1.0.0
maiconda/snooker-frontend:v1.0.0
postgres:15-alpine
minio/minio:latest
minio/mc:latest
nats:alpine
```

As imagens da aplicacao estao versionadas. Antes de usar isso em producao real, fixe tambem tags especificas para MinIO, MinIO Client e NATS.

## Pre-requisitos no homelab

1. K3s instalado no notebook Debian.
2. StorageClass `local-path` disponivel.
3. Traefik habilitado no K3s.
4. O host `snooker.local` apontando para o IP Tailscale do servidor.
5. Headlamp instalado separadamente para visualizacao do cluster.

Validacao rapida:

```bash
kubectl get nodes
kubectl get storageclass
kubectl get pods -A
```

## Secrets

O arquivo real `k8s/secret.yaml` foi gerado localmente a partir do `.env` e esta ignorado pelo Git. Para recriar em outro ambiente, copie:

```bash
cp k8s/examples/secret.example.yaml k8s/secret.yaml
```

Depois ajuste os valores em `stringData`.

## Aplicacao

Aplicar em ordem:

```bash
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/serviceaccounts.yaml
kubectl apply -f k8s/headlamp.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/secret.yaml
kubectl apply -f k8s/storage/
kubectl apply -f k8s/nats.yaml
kubectl apply -f k8s/jobs/profile-db-init-script.yaml
kubectl apply -f k8s/jobs/profile-db-init-job.yaml
kubectl apply -f k8s/jobs/minio-init-job.yaml
kubectl apply -f k8s/apps/
kubectl apply -f k8s/ingress.yaml
kubectl apply -f k8s/networkpolicies.yaml
```

## Validacao

```bash
kubectl -n snooker get pods
kubectl -n snooker get statefulsets
kubectl -n snooker get pvc
kubectl -n snooker get jobs
kubectl -n snooker get svc
kubectl -n snooker get ingress
kubectl -n snooker get networkpolicy
```

Logs uteis:

```bash
kubectl -n snooker logs deployment/auth
kubectl -n snooker logs deployment/profile
kubectl -n snooker logs deployment/game
kubectl -n snooker logs job/profile-db-init
kubectl -n snooker logs job/minio-init
```

Inspecao para apresentacao:

```bash
kubectl -n snooker describe statefulset auth-postgres
kubectl -n snooker describe ingress snooker
kubectl -n snooker describe networkpolicy default-deny-all
kubectl -n snooker describe job profile-db-init
```

## Acesso

No computador cliente, aponte `snooker.local` para o IP Tailscale do notebook servidor. No Windows, edite como administrador:

```txt
C:\Windows\System32\drivers\etc\hosts
```

Exemplo:

```txt
100.x.y.z snooker.local
100.x.y.z headlamp.snooker.local
```

Depois acesse:

```txt
http://snooker.local
```

### Token do Headlamp

Para acessar o painel do Headlamp e visualizar o estado do cluster, utilize o token do administrador criado pelo manifesto `k8s/headlamp.yaml`. 

Obtenha o token executando o comando abaixo no terminal do servidor:

```bash
kubectl -n snooker get secret headlamp-admin-token -o jsonpath='{.data.token}' | base64 --decode
```

Copie o token gerado e cole-o na tela de login do Headlamp.

## Observacoes

- `COOKIE_SECURE=false` porque o plano atual usa HTTP local. Quando houver HTTPS, altere para `true`.
- O frontend ja foi compilado com o Google Client ID, mas o backend `auth` tambem recebe `GOOGLE_CLIENT_ID` em runtime.
- Os bancos foram mantidos separados para preservar os boundaries do projeto.
- O NetworkPolicy aplica default-deny e libera apenas os fluxos necessarios entre frontend, APIs, bancos, NATS e MinIO.
