#!/bin/bash

# ==============================================================================
# Script de automação para Build e Push de Imagens Docker para o Docker Hub (Bash)
# ==============================================================================

# Encerrar o script em caso de erro
set -e

# 1. Carregar variáveis do arquivo .env se ele existir
google_client_id=""
if [ -f .env ]; then
    echo "Carregando configurações de .env..."
    # Extrair GOOGLE_CLIENT_ID ignorando comentários e aspas
    google_client_id=$(grep -E "^GOOGLE_CLIENT_ID=" .env | cut -d'=' -f2- | tr -d '"'\')
fi

# 2. Solicitar o usuário do Docker Hub
read -p "Digite seu usuário do Docker Hub (ex: maiconda): " docker_user
if [ -z "$docker_user" ]; then
    echo -e "\e[31m[ERRO] Usuário do Docker Hub é obrigatório!\e[0m"
    exit 1
fi

# 3. Solicitar GOOGLE_CLIENT_ID se não encontrado no .env
if [ -z "$google_client_id" ]; then
    echo -e "\e[33mGOOGLE_CLIENT_ID não encontrado no arquivo .env.\e[0m"
    read -p "Digite o GOOGLE_CLIENT_ID para o build do Frontend (deixe em branco se não desejar configurar): " google_client_id
fi

# 4. Perguntar sobre autenticação no Docker Hub
read -p "Deseja realizar login no Docker Hub agora? (S/N): " login_needed
if [[ "$login_needed" =~ ^[Ss]$ ]]; then
    docker login
fi

echo -e "\n\e[32m=== Iniciando o processo de Build e Push ===\e[0m"

# --- Auth ---
auth_tag="${docker_user}/snooker-auth:v1.0.0"
echo -e "\n\e[36m[1/4] Construindo Auth: $auth_tag\e[0m"
docker build -t "$auth_tag" -f auth/Dockerfile .

echo -e "\e[36mEnviando Auth para o Docker Hub...\e[0m"
docker push "$auth_tag"

# --- Profile ---
profile_tag="${docker_user}/snooker-profile:v1.0.0"
echo -e "\n\e[36m[2/4] Construindo Profile: $profile_tag\e[0m"
docker build -t "$profile_tag" -f profile/Dockerfile .

echo -e "\e[36mEnviando Profile para o Docker Hub...\e[0m"
docker push "$profile_tag"

# --- Game ---
game_tag="${docker_user}/snooker-game:v1.0.0"
echo -e "\n\e[36m[3/4] Construindo Game: $game_tag\e[0m"
docker build -t "$game_tag" -f lobby/Dockerfile .

echo -e "\e[36mEnviando Game para o Docker Hub...\e[0m"
docker push "$game_tag"

# --- Frontend ---
frontend_tag="${docker_user}/snooker-frontend:v1.0.0"
echo -e "\n\e[36m[4/4] Construindo Frontend: $frontend_tag\e[0m"
if [ -n "$google_client_id" ]; then
    docker build --build-arg GOOGLE_CLIENT_ID="$google_client_id" -t "$frontend_tag" -f frontend/Dockerfile .
else
    docker build -t "$frontend_tag" -f frontend/Dockerfile .
fi

echo -e "\e[36mEnviando Frontend para o Docker Hub...\e[0m"
docker push "$frontend_tag"

echo -e "\n\e[32m🎉 Todos os containers foram criados e publicados com sucesso no Docker Hub!\e[0m"
echo -e "  - \e[90m$auth_tag\e[0m"
echo -e "  - \e[90m$profile_tag\e[0m"
echo -e "  - \e[90m$game_tag\e[0m"
echo -e "  - \e[90m$frontend_tag\e[0m"
