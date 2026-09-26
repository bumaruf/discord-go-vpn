package tasklist

import "testing"

func TestHas(t *testing.T) {
	discord := []string{"Discord.exe", "DiscordCanary.exe", "DiscordPTB.exe", "Vesktop.exe"}
	wiresock := []string{"wiresock-client.exe"}

	tests := []struct {
		name   string
		output string
		names  []string
		want   bool
	}{
		{
			name: "stable",
			output: `"System Idle Process","0","Services","0","8 K"
"Discord.exe","4812","Console","1","152,340 K"
`,
			names: discord,
			want:  true,
		},
		{
			name:   "canary",
			output: `"DiscordCanary.exe","5120","Console","1","140,112 K"` + "\n",
			names:  discord,
			want:   true,
		},
		{
			name:   "ptb",
			output: `"DiscordPTB.exe","6044","Console","1","139,876 K"` + "\n",
			names:  discord,
			want:   true,
		},
		{
			name:   "vesktop",
			output: `"Vesktop.exe","7300","Console","1","98,004 K"` + "\n",
			names:  discord,
			want:   true,
		},
		{
			name: "only other processes, including this client",
			output: `"explorer.exe","3100","Console","1","120,500 K"
"discord-go-vpn.exe","3900","Console","1","22,300 K"
`,
			names: discord,
			want:  false,
		},
		{
			name:   "no tasks",
			output: "INFO: No tasks are running which match the specified criteria.\r\n",
			names:  discord,
			want:   false,
		},
		{
			name:   "wiresock in CRLF output",
			output: "\"svchost.exe\",\"1200\",\"Services\",\"0\",\"9,876 K\"\r\n\"wiresock-client.exe\",\"8120\",\"Console\",\"1\",\"12,004 K\"\r\n",
			names:  wiresock,
			want:   true,
		},
		{
			name:   "wiresock not running",
			output: `"Discord.exe","4812","Console","1","152,340 K"` + "\n",
			names:  wiresock,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := has(tt.output, tt.names); got != tt.want {
				t.Errorf("has() = %v, want %v", got, tt.want)
			}
		})
	}
}
