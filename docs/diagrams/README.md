# Diagrama de uso

[PNG para o README](maestri-relay.png) · [HTML interativo](maestri-relay.html) · [Fonte JSON](maestri-relay.architecture.json)

O fluxo principal representa os comandos do Telegram até o Maestri. As respostas retornam pelo mesmo Relay. As ramificações mostram o catálogo, o provedor de IA opcional e o estado local. O desenho reflete `internal/app/relay*.go`, `internal/compose/relay.go` e `internal/adapters/maestri/`; não representa uma instalação real nem contém IDs ou credenciais de usuários.

O HTML pode ser baixado e aberto diretamente no navegador. Use os capítulos para destacar conversa ou criação, o botão de tema para alternar claro/escuro e Export para gerar PNG ou SVG. O PNG do README foi produzido por esse exportador. A fonte visual opcional vem do Google Fonts; sem rede, o navegador usa a fonte local de fallback. Texto do diagrama em português, controles fixos e `html lang` em inglês, conforme o suporte atual do Archify.

## Atualizar com Archify

Instale a skill [Archify](https://github.com/tt-a1i/archify). Defina `ARCHIFY_DIR` como a pasta do pacote instalado e, na raiz deste repositório, execute depois de editar o JSON:

```sh
node "$ARCHIFY_DIR/bin/archify.mjs" validate architecture docs/diagrams/maestri-relay.architecture.json --quality showcase --json
node "$ARCHIFY_DIR/bin/archify.mjs" deliver architecture docs/diagrams/maestri-relay.architecture.json docs/diagrams/maestri-relay.html --quality showcase --json
node "$ARCHIFY_DIR/bin/archify.mjs" visual-check docs/diagrams/maestri-relay.html --json
```

Depois da validação, abra o HTML, confira ambos os temas e exporte novamente o PNG. O GitHub exibe a imagem no README; o HTML precisa ser baixado ou hospedado separadamente para executar seus controles.

## Evidência desta versão

Archify 2.16, tipo `architecture`: validação e entrega com **9/9 verificações showcase, zero erros e zero avisos**; uma rodada de correção de posicionamento dos rótulos.

| Artefato | SHA-256 |
| --- | --- |
| JSON | `557e15255c87ab6492bc0ec2fe60ab0fa4d540e16528172f809e0e46428400cd` |
| HTML | `bb035ac38801a3b1b9e089f29ac088b2d3d83cb6d98880f4151cf6dc4352514b` |

Revisão visual: passou em Chromium via `agent-browser`, com capturas claras/escuras inspecionadas a 1440×900 e 2048×1320. A página estabilizada permaneceu dentro da viewport também a 1600×1000 e 1920×1080. A exportação PNG foi aberta e inspecionada.

O comando automatizado `visual-check` do pacote não completou nesta máquina, por timeout de `Page.loadEventFired`; seu resultado não é apresentado como aprovado. As medições e capturas acima foram feitas por uma sessão de navegador separada, sobre o mesmo HTML, sem modificar o artefato.
