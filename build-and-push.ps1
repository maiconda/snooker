# ==============================================================================
# Script de automação para Build e Push de Imagens Docker para o Docker Hub
# ==============================================================================

# 1. Carregar variáveis do arquivo .env se ele existir
$googleClientId = ""
if (Test-Path .env) {
    Write-Host "Carregando configurações de .env..." -ForegroundColor Gray
    Get-Content .env | ForEach-Object {
        # Ignorar comentários e linhas vazias
        if ($_ -and -not $_.StartsWith("#")) {
            if ($_ -match "^GOOGLE_CLIENT_ID=(.*)$") {
                $googleClientId = $Matches[1].Trim()
                # Remover possíveis aspas
                $googleClientId = $googleClientId -replace '^["'']|["'']$'
            }
        }
    }
}

# 2. Solicitar o usuário do Docker Hub
$dockerUser = Read-Host "Digite seu usuário do Docker Hub (ex: maiconda)"
if (-not $dockerUser) {
    Write-Host "[ERRO] Usuário do Docker Hub é obrigatório!" -ForegroundColor Red
    exit 1
}

# 3. Solicitar GOOGLE_CLIENT_ID se não encontrado no .env
if (-not $googleClientId) {
    Write-Host "GOOGLE_CLIENT_ID não encontrado no arquivo .env." -ForegroundColor Yellow
    $googleClientId = Read-Host "Digite o GOOGLE_CLIENT_ID para o build do Frontend (deixe em branco se não desejar configurar)"
}

# 4. Perguntar sobre autenticação no Docker Hub
$loginNeeded = Read-Host "Deseja realizar login no Docker Hub agora? (S/N)"
if ($loginNeeded -eq "S" -or $loginNeeded -eq "s") {
    docker login
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERRO] Falha ao realizar login no Docker Hub." -ForegroundColor Red
        exit 1
    }
}

Write-Host "`n=== Iniciando o processo de Build e Push ===" -ForegroundColor Green

# --- Auth ---
$authTag = "${dockerUser}/snooker-auth:v1.0.0"
Write-Host "`n[1/4] Construindo Auth: $authTag" -ForegroundColor Cyan
docker build -t $authTag -f auth/Dockerfile .
if ($LASTEXITCODE -ne 0) { Write-Host "[ERRO] Falha no build da imagem auth" -ForegroundColor Red; exit 1 }

Write-Host "Enviando Auth para o Docker Hub..." -ForegroundColor Cyan
docker push $authTag
if ($LASTEXITCODE -ne 0) { Write-Host "[ERRO] Falha no push da imagem auth" -ForegroundColor Red; exit 1 }

# --- Profile ---
$profileTag = "${dockerUser}/snooker-profile:v1.0.0"
Write-Host "`n[2/4] Construindo Profile: $profileTag" -ForegroundColor Cyan
docker build -t $profileTag -f profile/Dockerfile .
if ($LASTEXITCODE -ne 0) { Write-Host "[ERRO] Falha no build da imagem profile" -ForegroundColor Red; exit 1 }

Write-Host "Enviando Profile para o Docker Hub..." -ForegroundColor Cyan
docker push $profileTag
if ($LASTEXITCODE -ne 0) { Write-Host "[ERRO] Falha no push da imagem profile" -ForegroundColor Red; exit 1 }

# --- Game ---
$gameTag = "${dockerUser}/snooker-game:v1.0.0"
Write-Host "`n[3/4] Construindo Game: $gameTag" -ForegroundColor Cyan
docker build -t $gameTag -f lobby/Dockerfile .
if ($LASTEXITCODE -ne 0) { Write-Host "[ERRO] Falha no build da imagem game" -ForegroundColor Red; exit 1 }

Write-Host "Enviando Game para o Docker Hub..." -ForegroundColor Cyan
docker push $gameTag
if ($LASTEXITCODE -ne 0) { Write-Host "[ERRO] Falha no push da imagem game" -ForegroundColor Red; exit 1 }

# --- Frontend ---
$frontendTag = "${dockerUser}/snooker-frontend:v1.0.0"
Write-Host "`n[4/4] Construindo Frontend: $frontendTag" -ForegroundColor Cyan
if ($googleClientId) {
    docker build --build-arg GOOGLE_CLIENT_ID="$googleClientId" -t $frontendTag -f frontend/Dockerfile .
} else {
    docker build -t $frontendTag -f frontend/Dockerfile .
}
if ($LASTEXITCODE -ne 0) { Write-Host "[ERRO] Falha no build da imagem frontend" -ForegroundColor Red; exit 1 }

Write-Host "Enviando Frontend para o Docker Hub..." -ForegroundColor Cyan
docker push $frontendTag
if ($LASTEXITCODE -ne 0) { Write-Host "[ERRO] Falha no push da imagem frontend" -ForegroundColor Red; exit 1 }

Write-Host "`n🎉 Todos os containers foram criados e publicados com sucesso no Docker Hub!" -ForegroundColor Green
Write-Host "  - $authTag" -ForegroundColor Gray
Write-Host "  - $profileTag" -ForegroundColor Gray
Write-Host "  - $gameTag" -ForegroundColor Gray
Write-Host "  - $frontendTag" -ForegroundColor Gray
