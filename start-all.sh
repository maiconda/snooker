#!/bin/bash
# ==============================================================================
# Script para implantar todos os recursos seguindo a ordem correta dos manifestos
# ==============================================================================
set -euo pipefail

echo "========================================================"
echo "Iniciando implantação seguindo os manifestos do projeto..."
echo "========================================================"

# Aplica em ordem de dependência
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/serviceaccounts.yaml

# Garante a instalação do Headlamp via Helm antes de aplicar suas configurações adicionais (Ingress, RBAC)
echo "Instalando/Atualizando Headlamp via Helm..."
helm repo add headlamp https://kubernetes-sigs.github.io/headlamp/ --force-update || true
helm repo update
helm upgrade --install headlamp headlamp/headlamp --namespace snooker --set service.port=80

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

echo "========================================================"
echo "Todos os manifestos foram aplicados com sucesso!"
echo "Acompanhe os pods subindo com: kubectl get pods -n snooker -w"
echo "========================================================"
