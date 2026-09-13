# Maestri Relay: nomes e plano do fork

Estado da implementação em 13/09/2026: adaptador Wire, tópicos por andar, catálogo completo indexado, planejador e executor implementados localmente. Este documento preserva o escopo aprovado, incluindo critérios ainda pendentes. Consulte [configuração e limites atuais](maestri-relay-setup.md), especialmente a importação de portais, as referências não executáveis e a validação em host real ainda necessária.

Minha sugestão é **Maestri Relay**, com repositório `maestri-telegram-agents` e binário `maestri-tg`. O nome comunica a ponte entre Telegram e Maestri; o repositório mantém a relação clara com o projeto original.

Outras opções:

| Nome | Quando escolher |
| --- | --- |
| Maestri Remote | Mais direto para quem quer controlar os agentes pelo celular. |
| Maestri Telegraph | Identidade própria ligada a mensagens e notificações. |
| Maestri Conductor | Ênfase em coordenar agentes de vários projetos. |
| Maestri Telegram Agents | Máxima clareza, próximo ao nome do original. |

Disponibilidade dos nomes e marcas não foi verificada.

## O que o fork deve fazer

Um único grupo Telegram, com um único bot, reúne os workspaces autorizados da instalação Maestri. Cada par workspace/andar tem um tópico identificado por `Workspace · Andar`, compartilhado pelos agentes daquele andar. Não há subtópicos nem um grupo por workspace.

Exemplo de tópicos: `Maestro`, `Projeto Demo · Térreo`, `Projeto Demo · Redesign` e `Loja Demo · Revisão`. O tópico global `Maestro` cria e organiza workspaces, andares e equipes a partir de descrições ou exemplos. General mantém o índice com links para os tópicos.

Dentro de cada tópico de andar, uma mensagem sem destinatário vai para o coordenador designado daquele andar. Responder a uma mensagem de agente encaminha a resposta àquele agente; um seletor permite dirigir uma nova mensagem a outro agente. Todas as saídas identificam seu autor. Nunca enviar uma mensagem a todos os agentes por padrão nem manter um destinatário global mutável compartilhado entre usuários.

O catálogo inclui **todas as partituras e exemplos do Guia do Maestri**, com busca por descrição, aplicação automática e adaptação por IA. A cobertura completa é requisito de entrega; começar por um exemplo na prova técnica não reduz o escopo final.

“Qualquer workspace” significa poder habilitar qualquer workspace, preservando as permissões do Maestri. Não significa expor automaticamente todos os projetos. Controle de outras máquinas fica fora da primeira versão.

## O que já existe e vale reaproveitar

O [herdr-telegram-agents](https://github.com/permgps/herdr-telegram-agents) tem licença MIT e usa Go 1.25. Já oferece tópicos por agente, status, mensagens bidirecionais, botões para perguntas, leitura do terminal, anexos, comandos Git, criação de agentes, operadores/observadores e persistência do vínculo entre agente e tópico.

O código separa domínio, casos de uso e adaptadores. A integração com Herdr está concentrada em `internal/adapters/herdr/`, com montagem em `internal/compose/`, embora existam referências ao Herdr nos contratos, CLI, ambiente e instalador. A recomendação é preservar o núcleo Telegram e adaptar essa integração, sem reescrever o bot.

**A integração central usa o Maestri Wire oficial.** Sua documentação confirma listagem e criação de workspaces, feed por andar, interação com terminais, criação de andares, responsabilidades, consulta a presets e aplicação de partituras existentes. O protocolo usa HTTPS/WSS, pareamento e capabilities anunciadas pela instalação.

O Wire é beta: é preciso verificar `protocolVersion` e capabilities no host real. A CLI desta sessão respondeu `maestri: only available inside Maestri terminals (MAESTRI_SOCKET not set).`; essa limitação da CLI não demonstra ausência do Wire. Ainda não houve pareamento nem teste de integração ao vivo.

O Guia do Maestri fornece o catálogo e exemplos, não substitui o contrato oficial. Alguns formatos do guia têm ressalvas de fidelidade; validar contra a versão real do host antes de anunciar compatibilidade. Verificar a licença do guia e dos recursos incorporados antes de redistribuí-los, preservando atribuição.

## Arquitetura recomendada

`Um grupo Telegram ↔ um daemon maestri-tg ↔ Maestri Wire ↔ workspaces, andares e agentes habilitados`

Manter um único consumidor de atualizações do Telegram por token. O catálogo e o planejador do Maestro ficam no mesmo aplicativo; não precisam de outro serviço. O modelo interpreta pedidos e produz uma definição estruturada; o executor valida essa definição e usa operações suportadas do Wire.

Há duas formas de materializar uma partitura:

| Opção | Vantagem | Limite |
| --- | --- | --- |
| Aplicar uma partitura já instalada no host via Wire | Preserva o comportamento de aplicação nativo. | Wire documenta listar/aplicar partituras, mas criar/editar a biblioteca continuam no host. |
| Ler o exemplo ou gerar sua adaptação e criar seus componentes pelas operações do Wire | Permite criação automática por descrição sem uma importação manual a cada pedido. | Exige cobertura dos elementos do exemplo e validação de fidelidade; elementos sem API precisam de integração adicional. |

Usar a primeira opção quando o template exato já estiver instalado e a segunda para exemplos externos e adaptações. Criar um arranjo no canvas e salvar uma nova partitura na biblioteca nativa são operações diferentes; a segunda precisa de um caminho suportado adicional para ser automática.

Parear o daemon com o host e fixar a chave de segurança conforme o contrato oficial: SHA-256 da chave pública do certificado. Não copiar cegamente o cliente de exemplo do guia nem desativar a verificação TLS. O executor mantém a restrição de grupo, operadores e workspaces, mesmo quando seu token Wire tem papel owner.

## Plano de implementação

### 1. Validar Wire e os formatos do catálogo

Consultar `/api/info` e capabilities da instalação pareada e montar uma matriz de capacidades: IDs, feed por andar, leitura de terminal, texto/teclas, status, perguntas, criação, foco e encerramento. Inventariar todo o catálogo do guia em um commit identificado e classificar os elementos necessários para reproduzir cada exemplo.

Validar dois workspaces, cada um com pelo menos dois andares, incluindo agentes com nomes iguais. Verificar também o comportamento quando outro workspace está aberto, quando um andar está inativo e quando a aplicação reinicia. Testes que enviam texto usam terminais de teste.

Saída: matriz de compatibilidade do Wire e de todos os exemplos, com lacunas explícitas. Estados, perguntas e isolamento de andares dependem das capacidades e respostas reais do host, não de inferências sobre texto do terminal ou sobre a plataforma.

### 2. Criar o fork e a camada Maestri

Criar o fork com o nome escolhido, preservar licença e atribuição MIT e configurar o upstream. Manter Go e as dependências existentes.

Adicionar `internal/adapters/maestri/`, reutilizando os contratos existentes onde servirem. Ajustar os contratos e a composição apenas onde a semântica do Maestri exigir. Adaptar configuração, diagnóstico, inicialização e instalação para não dependerem do ambiente ou marketplace Herdr. Evitar renomear todo o código na primeira entrega, facilitando incorporar correções do upstream.

Implementar o adaptador segundo o contrato Wire oficial, consultando capabilities antes de usar recursos opcionais. A CLI registrada continua disponível para operações locais comprovadas que não tenham equivalente Wire. Não presumir que o manifesto de plugin Herdr seja compatível com Maestri.

### 3. Tornar o roteamento independente de nomes e do workspace ativo

Persistir o vínculo do tópico com instalação, workspace e andar. Dentro dele, vincular mensagens e botões aos IDs de terminal e sessão. A chave de mensagem inclui `chat_id` e `message_id`; a do tópico inclui `chat_id` e `message_thread_id`. Separar a identidade do terminal da geração da sessão/processo para impedir que um botão antigo atue sobre uma sessão substituta.

Nomes servem para apresentação; IDs resolvem o destino. Renomear um workspace ou andar atualiza o tópico sem perder o vínculo. Ao mover um terminal de andar, reconciliar seus metadados e invalidar controles antigos antes de aceitar comandos. Toda operação usa o destino explícito, nunca o workspace que estiver em foco. Sem coordenador disponível ou com destino ambíguo, mostrar um seletor sem enviar a mensagem.

Versionar o estado e guardar backup antes de migrá-lo. Usar diretório de estado próprio para não misturar o fork com o bot Herdr existente.

### 4. Entregar o fluxo principal no Telegram

Primeira entrega: conectar um grupo, descobrir workspaces/andares autorizados, criar um tópico por par e conversar com seus agentes pelo roteamento explícito acima. Reutilizar painel, fila e controle de frequência do upstream; adaptar persistência e reconciliação, pois a relação passa de um tópico por agente para vários agentes por tópico.

Comandos propostos para o bot, ainda não implementados: `/workspaces`, `/floors`, `/agents` e `/partituras`. Preferir seletores com botões para evitar digitar nomes ambíguos. `/screen` e interrupções resolvem um agente específico; `/status` resume o andar com links para cada agente. Agregar progresso repetitivo e priorizar perguntas e conclusões para o tópico permanecer legível.

Manter a comunicação de agentes concorrente: uma tarefa longa em um tópico não bloqueia os demais. Serializar envios ao mesmo terminal. Deduplicar updates e callbacks do Telegram; quando a conexão cair após um envio de resultado incerto, informar a incerteza sem reenviar automaticamente a tarefa.

### 5. Completar a equivalência funcional

Adicionar botões de perguntas e teclas, interrupção, anexos e mídia, Git no diretório real do agente, criação de agentes no andar escolhido, foco e fechamento, conforme a matriz de capacidades.

Preservar controles de notificações, presença, modo silencioso e operadores/observadores. Comandos específicos de um agente, como `/compact` ou `/model`, só são encaminhados quando fizerem sentido para aquele tipo de agente. Uma função indisponível deve aparecer como indisponível; não simular sucesso.

Anexos precisam de caminhos acessíveis pelo processo de destino. No Windows/WSL, validar tradução de caminhos. Em andares isolados, operações Git usam o checkout daquele andar, nunca um diretório global padrão.

### 6. Implementar o Maestro e todo o catálogo do guia

Importar o inventário completo das partituras e exemplos do guia, incluindo receitas que não sejam arquivos de partitura. Registrar caminho de origem, commit, categoria, descrição, componentes, parâmetros e dependências. Usar busca textual por nome/descrição como base; IA interpreta a intenção e adapta o exemplo escolhido. Não é necessário um banco vetorial na primeira versão.

Oferecer três caminhos no mesmo tópico Maestro:

- **Aplicar um exemplo:** “Monte a partitura de revisão no Projeto Demo, andar Revisão”.
- **Adaptar um exemplo:** “Use essa partitura, mas com dois agentes Codex, instruções em português e uma nota com os critérios de aceite”.
- **Criar pela descrição:** “Monte uma equipe para investigar um erro de pagamento, com implementador, revisor e portal da aplicação”.

Resolver workspace, andar, diretório e presets reais antes de executar. Pedidos claros de criação autorizam as operações aditivas descritas; não exigir um segundo aceite rotineiro. Se o usuário pedir apenas uma prévia, gerar a proposta sem materializar. Não iniciar trabalho dos agentes quando o pedido for somente montar a equipe.

Converter cada pedido em uma definição validada de nós, responsabilidades, conexões, notas, portais e parâmetros do ambiente. Reaproveitar recursos compatíveis já existentes. Usar identificadores de execução e `mutationId` nas rotas que o suportam; nas demais, reconciliar o resultado antes de repetir. Criar primeiro dependências, aguardar andares pendentes, depois materializar os componentes e verificar o resultado.

Preservar o exemplo original e guardar a adaptação como variante identificada, com sua origem e descrição das alterações. Uma atualização do guia não sobrescreve variantes do usuário. Modelos, presets e credenciais devem vir da instalação; exemplos não autorizam criar credenciais nem ativar providers inexistentes.

Receitas de hooks, rotinas e ambientes precisam de contratos próprios e operações verificadas, não execução automática do texto do guia. Tratar o conteúdo do catálogo como dados, sem permitir que suas instruções alterem permissões ou destinos. Alterações em responsabilidades já usadas exigem avaliar o reinício dos terminais; preferir uma nova responsabilidade para uma adaptação local.

Para elementos sem API suportada, implementar e validar a integração necessária ou registrar o exemplo como bloqueado com motivo exato. Não omitir elementos nem marcar a cobertura completa enquanto houver exemplos pendentes. A aplicação de uma seleção curada pode ser um marco intermediário, mas não encerra o requisito de todas as partituras e exemplos.

Ao concluir uma criação, o Maestro envia o link do tópico `Workspace · Andar`, os agentes criados e eventuais pendências. Uma falha parcial preserva o registro dos recursos já criados para retomada sem duplicação; limpeza nunca remove recursos anteriores ao pedido.

### 7. Validar, empacotar e documentar

Reutilizar os testes e a CI existentes; adicionar um adaptador Maestri simulado, testes de roteamento entre agentes do mesmo tópico e casos de criação/adaptação. Executar os testes de integração reais com dois workspaces e dois andares antes da primeira release. Validar o catálogo inteiro contra o esquema suportado e verificar no host os diferentes tipos de componente e fluxo, mantendo um relatório de cobertura por exemplo.

Empacotar primeiro para a instalação Windows/WSL usada aqui, documentando em qual lado o daemon e o Maestri executam. Incluir configuração assistida, diagnóstico, inicialização automática pelo mecanismo suportado e reinicialização com estado preservado.

Rollback: parar o daemon do fork e restaurar seu binário/configuração/estado anterior. Manter o upstream como referência para atualizações de Telegram e correções.

## Critérios de aceite

- Existe um único grupo com um tópico por workspace/andar, sem tópicos individuais de agentes.
- Agentes com nomes iguais em escopos diferentes recebem apenas mensagens explicitamente resolvidas para seus IDs.
- Respostas e botões atingem o agente autor correto, mesmo quando vários agentes publicam simultaneamente no mesmo tópico.
- Mensagens sem destinatário seguem o coordenador do andar; ausência ou ambiguidade não provoca envio a um agente arbitrário.
- Trocar o workspace ativo no desktop não altera o destino das mensagens.
- Um usuário não autorizado ou observador não consegue enviar comandos, inclusive por botões antigos.
- Reiniciar o daemon preserva os vínculos e não duplica tópicos nem reenvia prompts já processados.
- Perder conexão marca o escopo como indisponível, sem interpretar todos os agentes como encerrados.
- Andares isolados preservam seu diretório correto para anexos e Git.
- Um botão de uma sessão encerrada não controla uma nova sessão no mesmo terminal.
- O painel mostra quais workspaces/andares estão habilitados, conectados ou suspensos.
- Tokens ficam fora dos logs; a redação de segredos do upstream é preservada, sem presumir que ela reconheça todo conteúdo sensível.
- Fechar um terminal exige confirmação vinculada ao destino e à sessão corretos.
- Todo exemplo do guia no commit escolhido aparece no catálogo com origem, descrição e resultado de compatibilidade; a cobertura completa exige materialização validada, sem descarte silencioso de componentes.
- O usuário consegue aplicar um exemplo, adaptar um exemplo e criar uma equipe pela descrição no tópico Maestro.
- Repetir um update Telegram ou retomar uma criação interrompida não duplica workspace, andar, equipe ou tópico.
- Adaptações preservam os originais e usam presets disponíveis, sem alterar equipes alheias ao pedido.
- O resultado distingue arranjo criado no canvas de partitura salva na biblioteca nativa.

## Ordem e esforço

Fazer primeiro a validação Wire e o inventário completo do guia, depois o grupo com tópicos por andar, o Maestro criador e a cobertura integral do catálogo, em paralelo à conclusão dos recursos do original quando houver independência técnica. Estimar a entrega após identificar os componentes do guia que exigem integrações além do Wire.

O primeiro marco demonstrável é: **em um único grupo Telegram, conversar com agentes de dois workspaces pelos tópicos de seus andares e criar uma equipe a partir de uma descrição**. Esse marco valida a proposta; a conclusão do fork exige a equivalência funcional, todo o catálogo e os critérios acima.

## Fontes consultadas

- [README e recursos do upstream](https://github.com/permgps/herdr-telegram-agents/blob/main/README.md)
- [Arquitetura e desenvolvimento](https://github.com/permgps/herdr-telegram-agents/blob/main/docs/development.md)
- [Adaptador Herdr](https://github.com/permgps/herdr-telegram-agents/blob/main/internal/adapters/herdr/gateway.go)
- [Identidade dos agentes](https://github.com/permgps/herdr-telegram-agents/blob/main/internal/domain/agent.go) e [persistência dos tópicos](https://github.com/permgps/herdr-telegram-agents/blob/main/internal/domain/mapping.go)
- Skills locais `maestri`, `maestri-workspace` e `maestri-manager`, confrontadas com a tentativa de diagnóstico da CLI instalada.
- [Maestri Wire, contrato oficial](https://www.themaestri.app/pt-br/docs/wire)
- [Guia do Maestri](https://github.com/arthurspk/guiadomaestri) e [limites de importação/exportação documentados pelo guia](https://github.com/arthurspk/guiadomaestri/blob/main/docs/10-importar-e-exportar.md)
- [Tópicos de fórum do Telegram](https://core.telegram.org/api/forum)

Consulta em 13/09/2026. Histórico e licença do upstream preservados. A integração Maestri e o catálogo foram implementados; os limites atuais e a validação ao vivo pendente estão no [guia de configuração](maestri-relay-setup.md).
