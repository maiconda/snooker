#!/bin/bash
# ==============================================================================
# Script para remover todos os recursos criados com base nos manifestos do projeto
# ==============================================================================
set -euo pipefail

echo "========================================================"
echo "Derrubando recursos seguindo os manifestos do projeto..."
echo "========================================================"

# Deleta em ordem reversa de dependência
kubectl delete -f k8s/networkpolicies.yaml --ignore-not-found=true
kubectl delete -f k8s/ingress.yaml --ignore-not-found=true
kubectl delete -f k8s/apps/ --ignore-not-found=true
kubectl delete -f k8s/jobs/minio-init-job.yaml --ignore-not-found=true
kubectl delete -f k8s/jobs/profile-db-init-job.yaml --ignore-not-found=true
kubectl delete -f k8s/jobs/profile-db-init-script.yaml --ignore-not-found=true
kubectl delete -f k8s/nats.yaml --ignore-not-found=true
kubectl delete -f k8s/storage/ --ignore-not-found=true
kubectl delete -f k8s/secret.yaml --ignore-not-found=true
kubectl delete -f k8s/configmap.yaml --ignore-not-found=true
kubectl delete -f k8s/headlamp.yaml --ignore-not-found=true
kubectl delete -f k8s/serviceaccounts.yaml --ignore-not-found=true
kubectl delete -f k8s/namespace.yaml --ignore-not-found=true

echo "========================================================"
echo "Todos os recursos foram removidos com sucesso!"
echo "Para subir tudo novamente execute: ./start-all.sh"
echo "========================================================"
