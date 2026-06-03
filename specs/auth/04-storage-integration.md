# Especificação Técnica: Integração de Object Storage (MinIO / S3) para Avatares

Este documento especifica a integração com a API S3 (utilizando MinIO local ou AWS S3 em produção) para gerenciar o upload de imagens de perfil do jogador.

---

## 1. Variáveis de Configuração Requeridas

A inicialização da conexão com o Object Storage deve ler as seguintes variáveis de ambiente (gerenciadas e injetadas por K8s Secrets e ConfigMaps):

```env
STORAGE_ENDPOINT=minio-service.snooker-db.svc.cluster.local:9000
STORAGE_ACCESS_KEY=minioAdminAccessKey
STORAGE_SECRET_KEY=minioAdminSecretKey
STORAGE_USE_SSL=false
STORAGE_BUCKET_NAME=snooker-profiles
```

---

## 2. Fluxo e Convenção de Nome de Arquivos (Keys)

Para organizar os dados no bucket e garantir a limpeza automática de arquivos descartados ou não finalizados, dividimos a estrutura em dois prefixos lógicos:

### 2.1. Prefixo Temporário (Upload do Cliente)
*   **Formato da Chave:** `temp/<uuid>.png`
*   **Geração:** O UUID é gerado pelo backend em Go na chamada `GET /profile/upload-url`.
*   **Uso:** O cliente envia a imagem direto para este caminho usando o Presigned URL.
*   **Ciclo de Vida (Lifecycle Rule):** Configurado diretamente no MinIO/S3 para **expirar e deletar automaticamente** qualquer arquivo com prefixo `temp/` que tenha mais de 24 horas.

### 2.2. Prefixo Ativo (Perfis Definitivos)
*   **Formato da Chave:** `active/<user_id>.png`
*   **Mecanismo:** Ao receber o POST bem-sucedido em `/api/v1/profile/complete`, o Go faz uma cópia direta (S3 CopyObject) do arquivo de `temp/<uuid>.png` para `active/<user_id>.png`. Em seguida, deleta o arquivo em `temp/<uuid>.png`.
*   **URL Pública:** O link final persistido na tabela `perfis` será estruturado como: `https://<domain>/snooker-profiles/active/<user_id>.png`.

---

## 3. Algoritmo de Geração da Presigned URL

O método no Go deve assinar uma operação HTTP **PUT** válida por no máximo **5 minutos**. O cabeçalho de tipo de mídia deve ser validado.

```go
package storage

import (
	"context"
	"time"
	"github.com/minio/minio-go/v7"
)

type StorageService struct {
	Client     *minio.Client
	BucketName string
}

func (s *StorageService) GeneratePresignedUploadURL(ctx context.Context, objectName string) (string, error) {
	// Força o tipo de mídia a ser estritamente PNG ou JPEG
	reqParams := make(url.Values)
	
	// Gera URL pré-assinada para operação de PUT (Upload) válida por 5 minutos
	presignedURL, err := s.Client.PresignedPutObject(ctx, s.BucketName, objectName, time.Duration(5)*time.Minute)
	if err != nil {
		return "", err
	}
	
	return presignedURL.String(), nil
}
```

---

## 4. Critérios de Aceitação e Testes do Storage

Para validar a implementação da camada de armazenamento de arquivos:
1.  **Validação de Assinatura:** O teste de integração deve tentar realizar o upload via HTTP PUT com a URL pré-assinada gerada. Deve retornar `200 OK`.
2.  **Tentativa de Escrita Sem Assinatura:** Realizar um PUT direto no bucket sem os parâmetros de consulta da assinatura deve retornar obrigatoriamente `403 Forbidden`.
3.  **Tipo MIME Inválido:** Se configurados cabeçalhos de tipo de mídia restritos na assinatura, o upload de arquivos de formato inválido (ex: `.txt`, `.exe`) deve falhar ou ser mitigado no upload.
4.  **Cópia Interna S3:** O teste deve validar a cópia de objetos temporários para a pasta definitiva no backend e a exclusão da chave antiga temporária após o sucesso.
5.  **Configuração de Lifecycle no Setup:** Scripts de infra/setup devem garantir que a regra de ciclo de vida de 24 horas no prefixo `temp/` esteja ativa.
