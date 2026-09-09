package rtcpeer

import "testing"

func TestValidateICEAdvertiseIPs(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		wantErr bool
	}{
		{name: "empty", values: nil},
		{name: "private IPv4", values: []string{"10.0.0.1", "172.16.0.1", "192.168.0.1"}},
		{name: "Tailscale IPv4", values: []string{"100.64.0.1", "100.127.255.254"}},
		{name: "public IPv4", values: []string{"8.8.8.8"}, wantErr: true},
		{name: "outside Tailscale range", values: []string{"100.128.0.1"}, wantErr: true},
		{name: "IPv6", values: []string{"fd7a:115c:a1e0::1"}, wantErr: true},
		{name: "invalid", values: []string{"not-an-ip"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateICEAdvertiseIPs(tt.values)
			if tt.wantErr && err == nil {
				t.Fatal("validateICEAdvertiseIPs() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateICEAdvertiseIPs() error = %v, want nil", err)
			}
		})
	}
}
