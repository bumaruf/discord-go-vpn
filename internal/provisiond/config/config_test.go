package config

import "testing"

func TestLoadConfig(t *testing.T) {
	for name, value := range map[string]string{
		"VPN_PANEL_SECRET": "panel-secret",
		"VPN_LIVE_SECRET":  "live-secret",
		"PANEL_ORIGIN":     "https://panel.example.com",
		"MONGODB_URI":      "mongodb://localhost:27017",
		"MONGODB_DB":       "vpn_test",
		"WG_SUBNET":        "10.8.0.0/24",
	} {
		t.Setenv(name, value)
	}

	want := Config{
		PanelSecret:   "panel-secret",
		LiveSecret:    "live-secret",
		PanelOrigin:   "https://panel.example.com",
		MongoURI:      "mongodb://localhost:27017",
		MongoDatabase: "vpn_test",
		WGSubnet:      "10.8.0.0/24",
	}
	if got := Load(); got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestWGSubnetPrefix(t *testing.T) {
	for subnet, want := range map[string]string{
		"":             "10.9.0.0/24",
		"10.8.0.0/24":  "10.8.0.0/24",
		"10.8.0.1/24":  "",
		"10.8.0.0/16":  "",
		"fd00::/120":   "",
		"not a subnet": "",
	} {
		got, err := Config{WGSubnet: subnet}.WGSubnetPrefix()
		if want == "" {
			if err == nil {
				t.Errorf("WGSubnetPrefix(%q) = %v, want an error", subnet, got)
			}
			continue
		}
		if err != nil || got.String() != want {
			t.Errorf("WGSubnetPrefix(%q) = %v, %v; want %s", subnet, got, err, want)
		}
	}
}
