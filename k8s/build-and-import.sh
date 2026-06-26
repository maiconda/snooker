#!/bin/bash
# ==============================================================================
# Script para compilar as imagens e importar diretamente no containerd do K3s
# ==============================================================================
set -euo pipefail

# Configurações padrão
TAG="v1.0.0"
GOOGLE_CLIENT_ID="696708990480-d4mu1n3pjh98hab15cq1cmcuomta2usu.apps.googleusercontent.com"

echo "========================================================"
echo "1. Iniciando compilação das imagens da aplicação..."
echo "========================================================"

# Auth
echo "[1/4] Compilando Auth Service..."
docker build -t maiconda/snooker-auth:$TAG -f auth/Dockerfile .

# Profile
echo "[2/4] Compilando Profile Service..."
docker build -t maiconda/snooker-profile:$TAG -f profile/Dockerfile .

# Game/Lobby
echo "[3/4] Compilando Game/Lobby Service..."
docker build -t maiconda/snooker-game:$TAG -f lobby/Dockerfile .

# Frontend
echo "[4/4] Compilando Frontend (Google Client ID: $GOOGLE_CLIENT_ID)..."
docker build --build-arg GOOGLE_CLIENT_ID="$GOOGLE_CLIENT_ID" -t maiconda/snooker-frontend:$TAG -f frontend/Dockerfile .

echo "========================================================"
echo "2. Importando imagens no runtime do K3s (containerd)..."
echo "========================================================"

docker save maiconda/snooker-auth:$TAG | sudo k3s ctr images import -
docker save maiconda/snooker-profile:$TAG | sudo k3s ctr images import -
docker save maiconda/snooker-game:$TAG | sudo k3s ctr images import -
docker save maiconda/snooker-frontend:$TAG | sudo k3s ctr images import -

echo "========================================================"
echo "Sucesso! Imagens compiladas e carregadas no K3s."
echo "Agora você pode aplicar os manifestos com o kubectl."
echo "========================================================"
