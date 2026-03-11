package linux

import "testing"

func TestParseSmartctlFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name: "extract family line",
			output: `smartctl 7.5

=== START OF INFORMATION SECTION ===
Model Family:     Seagate Archive HDD (SMR)
Device Model:     ST8000AS0002-1NA17Z
Serial Number:    Z123DGBG
`,
			want: "Seagate Archive HDD (SMR)",
		},
		{
			name: "missing family line",
			output: `smartctl 7.5

=== START OF INFORMATION SECTION ===
Device Model:     ST8000AS0002-1NA17Z
`,
			want: "",
		},
		{
			name:   "collapses excess spaces",
			output: "Model Family:   HGST   Ultrastar HC310/320   \n",
			want:   "HGST Ultrastar HC310/320",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseSmartctlFamily([]byte(tt.output))
			if got != tt.want {
				t.Fatalf("parseSmartctlFamily() = %q, want %q", got, tt.want)
			}
		})
	}
}
