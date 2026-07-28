# CYD Solar

Mono-repo do display de energia solar off-grid. Tasks `CYDSOL-1..10` no planner.

**Fonte única de verdade:** a nota **"Spec CYD Solar"**, id
`818a6937-bb14-4b85-8568-d4b12956e03a`. As tasks referenciam §1..§9 dela.
A spec NÃO é duplicada aqui — busque via MCP na hora de usar. Esse fluxo
(nota + UUID) é instrução operacional pro Claude Code — não deve vazar pros
arquivos do repo (ver "Repositório é público" abaixo).

Requer o **MCP planner conectado**. Sem ele, pare e avise.

## Fluxo: "Implementar a task CYDSOL-N"

1. `get_task_by_code("CYDSOL-N")` — título e corpo: o que entrega, o Done, e
   quais § da spec ela referencia.
2. `get_note("818a6937-bb14-4b85-8568-d4b12956e03a")` — ler as seções
   referenciadas pela task.
3. **Antes de codar**, validar que a task dá pra executar como está escrita.
   Pare e pergunte se houver: ambiguidade real, conflito entre task e spec,
   placeholder ⚠️ não preenchido de que a task depende, ou dependência não
   satisfeita (task anterior não entregue). Não improvisar em cima de buraco.
4. Implementar em `API/` ou `CYD/` conforme a task, seguindo task + spec +
   convenções abaixo.
5. Conferir o resultado contra o **Done** da task, item por item.
6. Depois de commitar: **sempre** `submit_for_review(id, ref=<commit SHA>)`.
   Isso não é condicionado a Done 100% satisfeito — se algum item ficou
   pendente (dependência não entregue, verificação que exige acesso que o
   Claude Code não tem, etc.), isso vai no `create_log` e no report do passo
   7; quem decide se aquilo trava a review é o Marcos, não eu. Sempre que
   houver múltiplas subtasks na mesma leva de trabalho, `submit_for_review`
   em cada uma. **Nunca** `complete_task` — quem conclui é o Marcos, na UI.
7. Reportar: o que foi feito, como verificar o Done, e qualquer
   divergência/decisão tomada no caminho.

## Regras (fronteira)

- **Nunca editar a nota de Spec.** Se a implementação indicar mudança na spec
  (ex.: limiares de `level` da CYDSOL-1) → **propor** ao Marcos; ele consolida.
- **Nunca marcar task como done.** Só `submit_for_review`.
- **Verdade = a nota de Spec, buscada na hora.** Não duplicar conteúdo dela
  neste arquivo nem no código como comentário-cópia.
- **Dúvida real → perguntar.** Calibrar: perguntar sobre ambiguidade, conflito
  ou informação faltante. Não travar em escolha rotineira que a spec deixou
  aberta de propósito — decidir, seguir, e reportar a decisão no passo 7.

## Estrutura e convenções

```
API/   serviço Go (roda no VPS)
CYD/   firmware PlatformIO + TFT_eSPI (ESP32-2432S028)
```

**`API/`** — Go. Domínio separado de infra, sem cerimônia DDD em ferramenta
pequena: simples e modular. Desenhar antes de implementar.

**`CYD/`** — PlatformIO + TFT_eSPI. Pinagem e config do driver por **build
flags** no `platformio.ini` — nunca editar `User_Setup.h` da lib. Comandos
`pio` rodam a partir de `CYD/` (`pio run -d CYD`, ou `make build` dentro de
`CYD/`). `CYD/include/secrets.h` é gitignored — nunca commitar.

## Repositório é público

Assuma um leitor externo, sem contexto nenhum do planner, da propriedade ou
de mim.

- **Comentário de código e doc versionada (README, docs) não citam `Spec §N`
  nem `CYDSOL-N`.** Se a razão vem da spec, escreva a razão inline e
  autocontida. Ex.: em vez de "Spec §5: datas absolutas", escreva algo como
  "cache entries are keyed by absolute date — a relative label goes stale
  silently when the device is offline across midnight".
- **Mensagem de commit PODE citar `CYDSOL-N`.** É convenção normal de
  rastreabilidade (igual ticket de Jira), fica no histórico e não no arquivo
  que a pessoa lê pra entender o código. A regra é sobre conteúdo versionado
  nos arquivos, não sobre o git log.
- Nada de endereço, coordenada, caminho de máquina ou hostname pessoal em
  arquivo versionado. Padrão é `secrets.h` / build flag / variável de
  ambiente.
- Idioma: **README, doc e comentário de código em inglês.** Só a spec no
  planner e as tasks continuam em português.
- Terceira pessoa ("aprovado pelo Marcos") lê estranho em repo público —
  reescrever de forma neutra ou em primeira pessoa.

**Segredos:** há um pre-commit hook (`.githooks/pre-commit`) que bloqueia
commits com segredo aparente (chaves, tokens, senhas, arquivos tipo
`secrets.h`/`.env`). Clone novo precisa rodar `scripts/setup-hooks.sh` uma
vez (seta `core.hooksPath`). Falso positivo: `# allow-secret` no fim da
linha, ou path em `.secretsallow`. Nunca contornar com `--no-verify` sem
confirmar com o Marcos que não é um segredo real.
