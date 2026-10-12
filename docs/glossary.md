# Glossário

Cada termo do domínio tem **uma** tradução em inglês, e ela vale igual em Go, TypeScript, MongoDB, rotas HTTP e env vars. Para nomear algo novo, procure o termo aqui primeiro. Um nome novo é combinado com o dono. Um conceito que não está na tabela entra como uma linha nova.

O texto que o usuário final lê continua em pt-BR (TUI, CLI, painel). O glossário vale só para os nomes no código e nos dados.

| pt-BR | English | O que é neste projeto |
|---|---|---|
| convite | invite | Todo acesso ao servidor: um código gerado no painel que viaja dentro do token de conexão. O servidor guarda só o hash SHA-256; o admin pode revogar e reativar. Tem um tipo (`kind`): individual ou global. |
| convite individual | individual invite | Convite de uma pessoa. Tipo `individual`, o padrão de um convite sem tipo gravado. |
| convite global, token global | global invite | Convite compartilhado por várias pessoas. Tipo `global`. A diferença para o individual é só de organização. |
| token de conexão | connection token | O que quem usa recebe e cola no cliente: `dgv1_` + base64url de `{server, fingerprint, invite}`. Leva o endereço do `provisiond`, a impressão digital do certificado e o convite. |
| dispositivo | device | Uma máquina e quem a usa: o nome do computador que o cliente envia mais o usuário do sistema, sem diferenciar maiúsculas. Fica dentro do último convite pelo qual a máquina entrou. |
| usuário do sistema | os user | A conta do sistema operacional que roda o Discord: `SUDO_USER` no Linux, o usuário da sessão da área de trabalho no Windows. |
| aluguel, acesso | lease | Peer WireGuard temporário que o `provisiond` concede a uma execução do cliente. Tem prazo de validade e é renovado enquanto o cliente roda. |
| sessão | session | Registro, no banco, de um lease em uso: tráfego, origem, último handshake. |
| limite de conexões | max leases | Quantos leases um convite pode ter ao mesmo tempo, definido no painel. Campo `maxLeases`; sem o campo, não há limite. |
| peer | peer | Peer WireGuard: uma chave pública com um endereço. |
| peer órfão | orphan peer | Peer que ficou na `wg0` sem lease, porque o `provisiond` reiniciou com o cliente conectado. |
| rótulo | label | Nome legível: o do dispositivo, enviado pelo cliente, ou o que o admin dá a um convite. |
| origem | origin | IP de onde o cliente se conecta (mascarado no painel até o admin pedir para ver). |
| consumo | usage | Bytes e tempo online somados por dia. |
| cota | quota | Limite mensal de tráfego. |
| amostra | sample | Ponto da série temporal de vazão (download/upload por segundo). |
| vazão | throughput | Taxa de transferência em bits por segundo. |
| coletor | collector | Laço do `provisiond` que lê `wg show dump` a cada segundo e alimenta o SSE e o banco. |
| painel | panel | A aplicação Next.js de administração. Na URL é `/dashboard`. |
| tíquete | ticket | Passe assinado e de vida curta que o painel emite para o navegador abrir o SSE do `provisiond`. |
| papel | role | Nível de acesso de quem está logado no painel (hoje só `admin`). |
| auditoria | audit log | Registro das ações do admin. |
| revogar, reativar | revoke, reactivate | Invalidar um convite e desfazer isso. |
| desconectar | disconnect | Derrubar um lease ativo. |
| servidor | server | A VPS com WireGuard e `provisiond`. Em amostras e consumo, é o escopo agregado. |
| usuário | user | Quem usa a VPN. Em amostras e consumo, é o escopo por identidade. |
| identidade | identity | Chave de agregação do consumo: o id do dispositivo, `peer:<chave>` quando não há banco para registrá-lo, ou o servidor. |
| sem nome | unnamed | Dispositivo cujo cliente não enviou rótulo. |
| adotar | adopt | Trazer para a chave atual um dispositivo da mesma máquina gravado sob uma chave antiga (sem usuário do sistema, ou sob outro convite, inclusive o nulo que o token global deixou), preenchendo o que falta nela. O consumo continua no mesmo id. |
| inativo | inactive | Dispositivo sem lease que não conecta há mais de 5 dias. A página "Quem está usando" o esconde até o admin pedir para ver. |
| túnel | tunnel | O túnel WireGuard por onde passa o Discord, levantado pelo transporte do cliente. |
| perfil | profile | Configuração WireGuard que o cliente usa: um `.conf` na pasta ou a que vem de um lease. |
| bandeja | tray | Ícone do cliente na área de notificação do Windows. |
