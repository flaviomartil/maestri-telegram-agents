# Usar o Maestri Relay

O Relay conecta um grupo Telegram a uma instalação Maestri. Cria o tópico `Maestro` e tópicos `Workspace · Andar` para os workspaces autorizados. Agentes do mesmo andar compartilham o tópico; responder ao card de um agente seleciona o destinatário por ID. Texto sem resposta exige um único coordenador ativo no andar.

## Preparar e conectar

Requer Go 1.25, Make, Maestri com Wire habilitado e um bot administrador de supergrupo Telegram com tópicos e permissão de gerenciá-los. Instale no host os presets dos agentes utilizados pelas partituras. Para os testes com detector de race, tenha também um compilador C.

```sh
make build
bin/maestri-tg init
```

Edite `~/.config/maestri-relay/config.json`; `init` cria um esqueleto sem sobrescrever arquivos existentes. Preencha:

| Campo | Valor |
| --- | --- |
| `wireURL` | Origem HTTPS exibida pelo host Wire, acessível pela máquina do Relay. |
| `wirePin` | SHA-256 da chave pública SPKI exibida pelo host, em hexadecimal ou base64. Confira diretamente no Maestri. |
| `chatID` | ID negativo do único supergrupo Telegram. |
| `operators` | IDs numéricos positivos dos usuários que podem controlar os agentes. |
| `observers` | IDs de usuários que podem consultar `/help` e `/status` no General. |
| `workspaces` | IDs dos workspaces autorizados. `[]` não autoriza nenhum; `["*"]` autoriza todos e permite criar novos. |
| `stateDir` | Diretório absoluto para estado e pareamento, exclusivo deste grupo/host. |
| `catalogDir` | Diretório absoluto para o catálogo baixado do guia. |
| `pollSeconds` | Intervalo de consulta; padrão 5 segundos. |
| `llmURL` | URL base compatível com OpenAI, por exemplo `https://api.openai.com/v1`; o cliente acrescenta `/chat/completions`. Opcional para aplicar modelos sem adaptação. |
| `llmModel` | Identificador de modelo disponível no provedor configurado. |

Forneça `TELEGRAM_BOT_TOKEN` no ambiente local do processo. Para IA, forneça `MAESTRI_LLM_KEY` conforme o provedor. Não coloque segredos no JSON nem no repositório.

Ative o pareamento no Maestri e execute:

```sh
bin/maestri-tg pair
bin/maestri-tg doctor
bin/maestri-tg catalog-sync
bin/maestri-tg run
```

`pair` solicita o código de seis dígitos e salva o token em `stateDir/wire-token`, com permissão `0600`. Alternativamente, forneça `MAESTRI_WIRE_TOKEN` no ambiente. Use pareamento `owner` para criação e controle. `doctor` verifica conexão, protocolo e papel; a compatibilidade de cada recurso depende das capabilities anunciadas pelo host.

`run` mantém o daemon no terminal; use seu supervisor habitual para operação contínua. Execute somente um consumidor Telegram por token. O lock local impede dois processos usando o mesmo `stateDir`, mas não protege instalações em máquinas ou diretórios diferentes. Os instaladores Herdr do upstream não instalam o Relay.

## Comandos no Telegram

| Comando | Comportamento |
| --- | --- |
| `/status`, `/workspaces`, `/floors` | Lista os andares conhecidos com links para tópicos. |
| `/agents` | Publica cards dos agentes do andar; responda ao card desejado. |
| `/screen` | Mostra a prévia textual disponível no feed. |
| `/focus` | Revela o nó no Maestri. |
| `/stop`, `/interrupt` | Envia Escape ou Ctrl+C ao terminal escolhido. |
| `/close` | Pede confirmação antes de encerrar o processo. |
| `/mute`, `/unmute` | Controla notificações daquele andar. |
| `/partituras descrição` | Busca no catálogo inteiro; mostra até 12 resultados por consulta. |
| `/apply ID` | Aplica o exemplo no andar do tópico atual. |
| `/adapt ID descrição` | Adapta o exemplo usando IA e cria a equipe. |
| `/create descrição` | Cria uma equipe por descrição. |
| `/preview descrição` | Devolve um plano JSON, sem criar recursos. |
| `/resume ID` | Retoma uma criação usando os resultados já registrados. |
| `/bind workspace-id floor-id\|ground topic-id` | Recupera manualmente um vínculo quando a criação de tópico teve resposta incerta. |

No `Maestro`, por exemplo: “Crie no workspace Projeto Demo um andar Revisão com um coordenador e dois revisores Codex”. Para criar workspace novo, informe também o diretório explicitamente e autorize `"*"` na configuração. Nos tópicos de andar, o destino fica preso ao tópico; pedidos para mudar o destino devem usar Maestro.

Perguntas e pedidos de proposta são tratados pelo planejador sem criação. Operadores podem criar recursos diretamente por descrição; o Relay valida o plano e só permite presets instalados. Prompts, descrições, nomes de workspaces e conteúdo do exemplo adaptado são enviados ao provedor de IA configurado. Arquivos de até 8 MiB podem ser enviados ao agente escolhido, incluindo legenda.

## Catálogo e portais

Fonte: [Guia do Maestri](https://github.com/arthurspk/guiadomaestri), commit `24ccb073c761864d3352552053a38240f70224a1`. `catalog-sync` baixa a revisão fixada: 257 partituras e 115 referências, total de 372 entradas. Não acompanha mudanças futuras até atualizar e validar a revisão.

Todas as partituras dessa revisão são convertidas, preservando os componentes de terminal, nota e portal, suas responsabilidades, posições e ligações. Comandos de lançamento do guia são substituídos pelos presets instalados. As outras 115 entradas ficam disponíveis como notas de referência, incluindo receitas, prompts, instruções e exemplos de temas/workspaces. Isso não implementa execução automática dessas receitas ou importação de configurações de tema/workspace.

Terminais e notas são criados pelas rotas Wire. Portais usam a aplicação nativa de uma partitura instalada. Para preparar toda a biblioteca:

```sh
bin/maestri-tg catalog-export --out /tmp/Relay.maestripartituras
```

Esse comando requer Wire pareado e presets instalados compatíveis com todos os agentes do catálogo. Importe o arquivo pela biblioteca de partituras do Maestri. Depois, `/apply ID` aplica os modelos. Se uma variante com portais ainda não estiver instalada, o bot entrega `Relay.maestripartitura` e o ID de criação: importe e execute `/resume ID`. Adaptar a composição ou alterar um preset pode gerar uma variante que requer nova importação.

A API Wire documenta listar/aplicar partituras, mas não importar, salvar ou editar a biblioteca nem criar um portal diretamente. Logo, criação inteiramente automática de toda variante com portais ainda não é possível por essa API. O guia não apresentou uma licença de redistribuição; seu conteúdo é baixado localmente, sem ser incorporado a este fork. Preserve a atribuição e verifique direitos antes de redistribuir os arquivos gerados.

## Planos pela CLI e recuperação

Todos os comandos aceitam `--config PATH`. Liste os IDs com `catalog-list --query descrição`. Um plano baseado em catálogo pode ser gerado sem Wire:

```sh
bin/maestri-tg plan --template ID --workspace WORKSPACE_ID --floor FLOOR_ID --out /tmp/equipe.json
bin/maestri-tg apply --plan /tmp/equipe.json --job revisao-001
```

Omita `--floor` para o térreo. Para planejamento por IA, use `plan --request "descrição"`; essa operação consulta os workspaces, andares e presets no Wire, mas não cria recursos. Revise o JSON. Um plano com `action: "preview"` precisa ser explicitamente alterado para `action: "create"` antes de `apply`; respostas e outras ações são recusadas.

Mantenha o mesmo `--job` e o mesmo plano para retomar uma criação. O executor registra cada etapa antes de enviar a mutação e cada resultado recebido antes de avançar. Um timeout após envio deixa a etapa incerta e bloqueia repetição automática. `mutationId` identifica a operação; não é tratado como garantia de idempotência. Nessa situação, confira o canvas e o registro `stateDir/relay.json` antes de qualquer recuperação manual. Não troque de job para contornar a incerteza, pois isso pode duplicar recursos.

Novos andares são criados sem isolamento Git; não há opção de criação de branch nesta versão. Cada execução de provisionamento tem prazo de cinco minutos. A verificação confirma IDs dos nós criados diretamente; na aplicação nativa, confirma novos nós por tipo em relação ao canvas anterior. Ela não comprova fidelidade visual ou identidade dos recursos diante de edições concorrentes no host.

## Validação e pendências

```sh
make test
make build
make lint
```

`make lint` requer Staticcheck compatível com Go 1.25 e verifica cinco alvos de compilação. Para validar o corpus completo já baixado:

```sh
MAESTRI_GUIDE_TEST_DIR="$HOME/.cache/maestri-relay/guide/24ccb073c761864d3352552053a38240f70224a1" go test -v ./internal/adapters/maestri -run TestFullGuideCatalog
```

Os testes cobrem TLS/SPKI, permissões, roteamento por IDs, respostas antigas, callbacks adulterados, falhas de persistência, retomada sem repetição e conversão do catálogo. Não houve pareamento nem envio de mensagens a um Telegram/Maestri real nesta entrega. Ainda é necessário validar dois workspaces com múltiplos andares, agentes homônimos, mudanças de foco e reinício no host utilizado.

O Relay ainda não reproduz todo o conjunto Herdr: histórico completo/emulação do terminal, comandos Git, agregação de álbuns, painel fixado, presença e todas as preferências do upstream ficam pendentes. `/screen` mostra uma prévia. A API fornece `epoch`, mas não uma precondição documentada de geração para cada processo reiniciado com o mesmo ID. Updates Telegram preservam a fila remota no início; o buffer de recepção permanece em memória, sem uma caixa de entrada durável. Mensagens já registradas são deduplicadas por até 48 horas; uma falha depois do registro e antes da entrega pode exigir reenvio explícito.
