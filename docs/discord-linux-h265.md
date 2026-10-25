# Stream com "Error: 2012" no Discord Linux (H265)

Sintoma: no Linux, ao **assistir** à stream de alguém, o Discord mostra
*Stream failed to start, Error: 2012*. Voz e a própria transmissão funcionam. A
VPN não tem parte nisso: a stream chega inteira pelo túnel, o Discord é que não
consegue decodificá-la.

## Causa

Desde o Discord 1.0.166, o cliente Linux anuncia que decodifica H265 (HEVC). O
servidor então manda a stream em H265 quando quem transmite suporta, e o cliente
falha ao criar o decodificador. Dá para confirmar em
`~/.config/discord/logs/discord-webrtc_0`:

```
video_coding.cpp: Could not create any video decoder, format: H265, from factories: WebRtcDecoderFactory
[stream] Inbound stats for user: … codec: H265 (NullVideoDecoder), … decoded: 0 …
```

Não há opção no Discord para desligar o H265, e o codec é escolhido pela
negociação com quem transmite.

## Solução: decodificar pela GPU NVIDIA

O [discord-linux-nvcodec-hook](https://github.com/TomoriLab/discord-linux-nvcodec-hook)
é uma biblioteca `LD_PRELOAD` que faz o módulo de voz do Discord decodificar
VP8, AV1, H264 e H265 pelo NVDEC. Precisa de GPU NVIDIA com `libcuda.so.1` e
`libnvcuvid.so.1`, e de Rust e Cargo para compilar (sem dependências externas).

```sh
git clone https://github.com/TomoriLab/discord-linux-nvcodec-hook.git
cd discord-linux-nvcodec-hook
./build.sh
./install.sh install
```

O `install.sh` deixa a biblioteca e o `launch.sh` em
`~/.local/share/discord-nvenc-nvdec-patch/` e cria
`~/.local/share/applications/discord.desktop` apontando para o `launch.sh`.
Nada é mudado no Discord nem no sistema.

### Como o discord-go-vpn entra nisso

O cliente abre o Discord do jeito que o menu da área de trabalho abriria: se
existe um `discord.desktop` seu em `~/.local/share/applications`, é o `Exec`
dele que roda dentro do túnel. Com o hook instalado, o próximo "conectar"
já abre o Discord pelo `launch.sh`, sem configurar nada no serviço.

A entrada precisa ter só o executável no `Exec` (códigos como `%U` são
ignorados). Uma entrada com argumentos não é usada, e o cliente procura o
Discord como antes. A opção `-discord caminho` continua valendo e tem
prioridade sobre a entrada.

### Conferindo

Conecte, abra uma stream e veja o decodificador em uso:

```sh
grep -oE "codec: H265 \([A-Za-z]*\).*decoded: [0-9]+" ~/.config/discord/logs/discord-webrtc_0 | tail -3
```

Com o hook funcionando aparece `H265 (nvdec)` e `decoded` maior que zero.

### Quando o Discord atualiza

O hook depende de partes privadas do módulo de voz, validadas contra um hash
específico dele. Depois de uma atualização do Discord, ele pode se desativar
sozinho (o Discord volta a funcionar como antes, sem H265) ou, no pior caso,
derrubar o Discord ao decodificar. Se a stream voltar a falhar, rode o `grep`
acima, confira o README do hook para a versão validada e recompile. Para
remover: `./install.sh uninstall`.
