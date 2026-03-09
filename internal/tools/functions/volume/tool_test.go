package volume

import "testing"

func TestToolRun(t *testing.T) {
	t.Parallel()

	tool := New()

	tests := []struct {
		name    string
		args    map[string]any
		want    int
		wantErr bool
	}{
		{
			name: "整数をそのまま受け付ける",
			args: map[string]any{"volume_percent": 50},
			want: 50,
		},
		{
			name: "JSON由来のfloat64整数値を受け付ける",
			args: map[string]any{"volume_percent": float64(75)},
			want: 75,
		},
		{
			name:    "1未満は拒否する",
			args:    map[string]any{"volume_percent": 0},
			wantErr: true,
		},
		{
			name:    "100超えは拒否する",
			args:    map[string]any{"volume_percent": 101},
			wantErr: true,
		},
		{
			name:    "小数は拒否する",
			args:    map[string]any{"volume_percent": 33.3},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tool.Run(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			gotPercent, ok := got["volume_percent"].(int)
			if !ok {
				t.Fatalf("volume_percent type = %T", got["volume_percent"])
			}
			if gotPercent != tt.want {
				t.Fatalf("volume_percent = %d, want %d", gotPercent, tt.want)
			}
		})
	}
}
