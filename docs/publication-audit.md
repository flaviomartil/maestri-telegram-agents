# Credenciais e publicação

Auditoria local em 13/09/2026. Não foram encontradas credenciais reais no código atual ou no histórico Git analisado. Isso é o resultado de uma inspeção, não uma garantia de ausência de todo segredo possível.

## O que foi verificado

- Gitleaks 8.30.1, com regras padrão e valores sensíveis ocultados nos relatórios: diretório atual e histórico de todas as refs, abrangendo 116 commits do upstream.
- Nomes de arquivos de credenciais, caminhos pessoais e identificadores de projetos nos arquivos candidatos a publicação, incluindo os ainda não rastreados pelo Git.
- Carregamento de `TELEGRAM_BOT_TOKEN`, `MAESTRI_WIRE_TOKEN` e `MAESTRI_LLM_KEY` pelo ambiente, sem tokens embutidos na configuração do Relay.

O scanner encontrou um único caso, tanto no diretório quanto no histórico: `internal/domain/redact_test.go`, linhas 34–35, introduzido no commit upstream `1bcbfa18f00d5dfb4d942753d4c2a770079f4f55`. É uma fixture de teste de redação com marcador de chave privada e conteúdo truncado, não uma chave criptográfica utilizável. O resultado foi inspecionado, não suprimido por uma regra ampla. Outros tokens de teste são valores sintéticos usados com servidores simulados.

A inspeção não autenticou tokens contra provedores nem consultou cofres de credenciais. Relatórios brutos e artefatos internos da auditoria ficam fora dos arquivos publicáveis.

## Preparação feita

Os exemplos próprios usam nomes genéricos de projeto. O caminho pessoal da máquina foi removido da documentação. O `.gitignore` agora cobre `.env`, `wire-token`, `relay.json`, configuração local, diretórios de estado/catálogo, partituras exportadas e instruções/memórias locais de agentes. O comportamento de ignorar não remove arquivos já rastreados; nenhum desses arquivos de credenciais estava rastreado nesta inspeção.

Cada usuário fornece seu próprio bot Telegram, pareamento Wire e, opcionalmente, provedor de IA. O padrão salva configuração e pareamento fora da pasta do repositório. O estado contém mensagens, planos e vínculos do usuário e também deve permanecer privado.

Código e licença MIT do upstream foram preservados. O Guia do Maestri é baixado localmente em revisão fixada: seu conteúdo e as partituras geradas não são redistribuídos pelo fork, pois uma licença de redistribuição do guia não foi localizada.

## Repetir antes de publicar uma nova versão

Com Gitleaks instalado, execute na raiz, mantendo os relatórios fora do repositório:

```sh
gitleaks git . --log-opts='--all' --redact --report-format json --report-path /tmp/relay-history.json
gitleaks dir . --redact --report-format json --report-path /tmp/relay-worktree.json
git status --short
git diff --check
```

O caso sintético descrito acima faz o Gitleaks retornar código 1; investigue qualquer diferença de arquivo, linha ou regra. Não interprete automaticamente todo resultado como falso positivo. Este documento não registra publicação: criação do repositório remoto e envio ao GitHub são etapas separadas.
